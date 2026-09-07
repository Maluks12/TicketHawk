package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Shortfall detection and recovery ──────────────────────────────────────────
//
// A wallet can only go negative through offline spend, so every function here
// is dead code for an event that never enabled it — which is most events.
//
// The reason a shortfall can happen at all, even with the Phase 4a caps working
// perfectly: a till's cached balance can be STALE-HIGH. The holder spent online
// at another stall while this till was dark, so the till approved against money
// that was already gone. That is not a control failure, it is the cost of
// operating offline at all, and it is why the caps bound the size rather than
// preventing the event.

// DetectShortfalls finds wallets that have gone negative and opens a row for
// each, or updates an existing one.
//
// Idempotent by design — the worker runs on a timer and a wallet that is still
// negative on the next pass must not accumulate a second shortfall row.
func DetectShortfalls(ctx context.Context, pool *pgxpool.Pool, eventID string) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT w.id, w.card_on_file,
		       COALESCE((SELECT SUM(amount) FROM cashless_transactions t WHERE t.wallet_id = w.id), 0)
		FROM cashless_wallets w
		WHERE w.event_id = $1
		  AND COALESCE((SELECT SUM(amount) FROM cashless_transactions t WHERE t.wallet_id = w.id), 0) < 0
	`, eventID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type neg struct {
		walletID   string
		cardOnFile bool
		balance    float64
	}
	found := []neg{}
	for rows.Next() {
		var n neg
		if err := rows.Scan(&n.walletID, &n.cardOnFile, &n.balance); err != nil {
			return 0, err
		}
		found = append(found, n)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	opened := 0
	for _, n := range found {
		cents, err := ToCents(-n.balance) // positive magnitude
		if err != nil {
			continue
		}

		// A cash-funded wallet going negative is flagged at DETECTION, not
		// later. §5: a correctly enforced zero offline cap makes this
		// impossible, so its existence proves a control was bypassed — and the
		// row should say so from the moment it is created, before any recovery
		// logic gets a chance to treat it as routine.
		fraud := !n.cardOnFile

		tag, err := pool.Exec(ctx, `
			INSERT INTO wallet_shortfalls (wallet_id, event_id, amount_cents, fraud_flag, state)
			VALUES ($1, $2, $3, $4, CASE WHEN $4 THEN 'investigating'::shortfall_state ELSE 'open'::shortfall_state END)
			ON CONFLICT (wallet_id, state) DO UPDATE
			SET amount_cents = EXCLUDED.amount_cents
			WHERE wallet_shortfalls.state IN ('open', 'recovering')
		`, n.walletID, eventID, cents, fraud)
		if err != nil {
			return opened, err
		}
		opened += int(tag.RowsAffected())
	}
	return opened, nil
}

// Shortfall is an open recovery case.
type Shortfall struct {
	ID             string
	WalletID       string
	EventID        string
	AmountCents    int64
	RecoveredCents int64
	State          string
	Attempts       int
	FraudFlag      bool
	MandateID      *string
	SettleSentAt   *time.Time
	DetectedAt     time.Time

	// Joined wallet facts the ladder needs.
	CardOnFile   bool
	BalanceCents int64
}

// ListActionableShortfalls returns cases the worker should act on.
//
// Excludes 'investigating' deliberately: those are findings for a human, and a
// worker that quietly retried them would be collecting on exactly the cases
// that indicate broken defences.
func ListActionableShortfalls(ctx context.Context, pool *pgxpool.Pool, limit int) ([]Shortfall, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := pool.Query(ctx, `
		SELECT s.id, s.wallet_id, s.event_id, s.amount_cents, s.recovered_cents,
		       s.state::text, s.attempts, s.fraud_flag, s.mandate_id,
		       s.settle_link_sent_at, s.detected_at,
		       w.card_on_file,
		       COALESCE((SELECT SUM(amount) FROM cashless_transactions t WHERE t.wallet_id = w.id), 0)
		FROM wallet_shortfalls s
		JOIN cashless_wallets w ON w.id = s.wallet_id
		WHERE s.state IN ('open', 'recovering', 'awaiting_settlement')
		ORDER BY s.detected_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Shortfall{}
	for rows.Next() {
		var s Shortfall
		var balance float64
		if err := rows.Scan(&s.ID, &s.WalletID, &s.EventID, &s.AmountCents,
			&s.RecoveredCents, &s.State, &s.Attempts, &s.FraudFlag, &s.MandateID,
			&s.SettleSentAt, &s.DetectedAt, &s.CardOnFile, &balance); err != nil {
			return nil, err
		}
		if c, err := ToCents(balance); err == nil {
			s.BalanceCents = c
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetActiveMandate loads a wallet's usable mandate, or nil.
func GetActiveMandate(ctx context.Context, pool *pgxpool.Pool, walletID string) (*Mandate, error) {
	m := &Mandate{}
	err := pool.QueryRow(ctx, `
		SELECT status::text, authorization_code, max_charge_cents, charged_cents, expires_at
		FROM payment_mandates
		WHERE wallet_id = $1 AND status = 'active'
		ORDER BY consent_at DESC
		LIMIT 1
	`, walletID).Scan(&m.Status, &m.AuthorizationCode, &m.MaxChargeCents,
		&m.ChargedCents, &m.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// CreateMandate records explicit consent to charge a card later.
//
// consentText is stored VERBATIM. "We showed them something like this" is not a
// defence, and this row is what answers a disputed recovery charge.
func CreateMandate(
	ctx context.Context, pool *pgxpool.Pool,
	walletID, eventID, userID, authCode, last4, brand string,
	maxChargeCents int64, consentText, consentVersion, ip string,
	expiresAt time.Time,
) (string, error) {
	if authCode == "" {
		// Refused rather than stored: a mandate with nothing to charge against
		// looks valid and fails weeks later, when the attendee is long gone.
		return "", errors.New("cannot create a mandate without a reusable authorization code")
	}
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO payment_mandates
			(wallet_id, event_id, user_id, authorization_code, card_last4, card_brand,
			 max_charge_cents, consent_text, consent_version, consent_ip, expires_at)
		VALUES ($1, $2, NULLIF($3,'')::uuid, $4, NULLIF($5,''), NULLIF($6,''),
		        $7, $8, $9, NULLIF($10,'')::inet, $11)
		RETURNING id
	`, walletID, eventID, userID, authCode, last4, brand,
		maxChargeCents, consentText, consentVersion, ip, expiresAt).Scan(&id)
	return id, err
}

// RecordRecoveryCharge applies a successful recovery to both the ledger and the
// mandate's running total, in one transaction.
//
// Both or neither. A charge that moved money without incrementing charged_cents
// would let the agreed ceiling be exceeded on the next pass, which is the exact
// promise the ceiling makes to the attendee.
func RecordRecoveryCharge(
	ctx context.Context, pool *pgxpool.Pool,
	shortfallID, walletID, eventID, mandateID string, amountCents int64,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`SELECT id FROM cashless_wallets WHERE id = $1 FOR UPDATE`, walletID); err != nil {
		return err
	}

	signed, err := SignedCents(TxnTopup, amountCents)
	if err != nil {
		return err
	}
	if _, err := appendEntry(ctx, tx, entryInput{
		ID: uuid.NewString(), WalletID: walletID, EventID: eventID,
		Kind: TxnTopup, SignedCents: signed,
		Detail: map[string]any{"method": "mandate_recovery", "mandate_id": mandateID},
	}); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE payment_mandates
		SET charged_cents = charged_cents + $2,
		    status = CASE WHEN charged_cents + $2 >= max_charge_cents
		                  THEN 'exhausted'::mandate_status ELSE status END
		WHERE id = $1
	`, mandateID, amountCents); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE wallet_shortfalls
		SET recovered_cents = recovered_cents + $2,
		    attempts = attempts + 1,
		    state = CASE WHEN recovered_cents + $2 >= amount_cents
		                 THEN 'recovered'::shortfall_state ELSE 'recovering'::shortfall_state END,
		    resolved_at = CASE WHEN recovered_cents + $2 >= amount_cents THEN now() ELSE NULL END,
		    mandate_id = $3
		WHERE id = $1
	`, shortfallID, amountCents, mandateID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordRecoveryFailure notes a failed attempt.
func RecordRecoveryFailure(ctx context.Context, pool *pgxpool.Pool, shortfallID, reason string) error {
	_, err := pool.Exec(ctx, `
		UPDATE wallet_shortfalls
		SET attempts = attempts + 1, last_error = $2, state = 'recovering'
		WHERE id = $1
	`, shortfallID, reason)
	return err
}

// MarkSettleLinkSent records that the attendee was asked to pay voluntarily.
func MarkSettleLinkSent(ctx context.Context, pool *pgxpool.Pool, shortfallID, token string) error {
	_, err := pool.Exec(ctx, `
		UPDATE wallet_shortfalls
		SET settle_link_sent_at = now(), settle_link_token = $2, state = 'awaiting_settlement'
		WHERE id = $1
	`, shortfallID, token)
	return err
}

// SetShortfallState moves a case without touching money.
func SetShortfallState(ctx context.Context, pool *pgxpool.Pool, shortfallID, state, note string) error {
	_, err := pool.Exec(ctx, `
		UPDATE wallet_shortfalls
		SET state = $2::shortfall_state,
		    last_error = NULLIF($3,''),
		    resolved_at = CASE WHEN $2 IN ('recovered','written_off') THEN now() ELSE resolved_at END
		WHERE id = $1
	`, shortfallID, state, note)
	return err
}