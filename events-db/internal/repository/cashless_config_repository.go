package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Offline configuration ─────────────────────────────────────────────────────
//
// CRUD for event_cashless_config. Until this existed, every safe default in
// Phase 4 assumed a row that nothing could create — an organiser could not
// enable offline spend without raw SQL on production.

// OfflineConfig is the full stored configuration, including the fields the
// enforcement path does not use but an operator needs to see and set.
type OfflineConfig struct {
	EventID             string      `json:"event_id"`
	Enabled             bool        `json:"enabled"`
	MaxOfflineTaps      int         `json:"max_offline_taps"`
	MaxOfflineSpend     float64     `json:"max_offline_spend"`
	MaxDistinctVendors  int         `json:"max_distinct_vendors"`
	DecaySteps          []DecayStep `json:"decay_steps"`
	UnknownTagAllowance float64     `json:"unknown_tag_allowance"`
	MaxRecoveryAttempts int         `json:"max_recovery_attempts"`
	UpdatedAt           *time.Time  `json:"updated_at,omitempty"`

	// Configured reports whether a row actually exists. Distinct from Enabled:
	// an event with NO row and an event explicitly set to disabled behave
	// identically at enforcement, but an operator needs to tell "never set up"
	// from "deliberately turned off" — the second is a decision, the first is
	// an omission.
	Configured bool `json:"configured"`
}

// GetOfflineConfig reads an event's configuration.
//
// A missing row returns Configured false and the safe defaults, NOT an error.
// The caller renders a form; there is nothing exceptional about an event that
// has never used offline spend, which is most of them.
func GetOfflineConfig(ctx context.Context, pool *pgxpool.Pool, eventID string) (*OfflineConfig, error) {
	c := &OfflineConfig{EventID: eventID}

	var (
		policy      string
		spendCents  int64
		allowCents  int64
		decayJSON   []byte
		updatedAt   time.Time
	)
	err := pool.QueryRow(ctx, `
		SELECT offline_policy::text, max_offline_taps, max_offline_spend_cents,
		       max_distinct_vendors, decay_steps, unknown_tag_allowance_cents,
		       max_recovery_attempts, updated_at
		FROM event_cashless_config WHERE event_id = $1
	`, eventID).Scan(&policy, &c.MaxOfflineTaps, &spendCents, &c.MaxDistinctVendors,
		&decayJSON, &allowCents, &c.MaxRecoveryAttempts, &updatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		// The §5 defaults, shown as a starting point. Enabled stays false —
		// presenting a form does not turn anything on.
		return &OfflineConfig{
			EventID: eventID, Configured: false, Enabled: false,
			MaxOfflineTaps: 3, MaxOfflineSpend: 50.00, MaxDistinctVendors: 2,
			DecaySteps: []DecayStep{
				{AfterMinutes: 10, MaxSpendCents: 2500},
				{AfterMinutes: 20, MaxSpendCents: 0},
			},
			UnknownTagAllowance: 5.00, MaxRecoveryAttempts: 3,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	c.Configured = true
	c.Enabled = policy == "enabled"
	c.MaxOfflineSpend = FromCents(spendCents)
	c.UnknownTagAllowance = FromCents(allowCents)
	c.UpdatedAt = &updatedAt
	if len(decayJSON) > 0 {
		_ = json.Unmarshal(decayJSON, &c.DecaySteps)
	}
	return c, nil
}

// UpsertOfflineConfig stores a validated configuration.
//
// Callers MUST run ValidateOfflinePolicy first. Not enforced here because the
// handler needs the full problem list to show an operator, and returning a
// []string from a repository write would be a strange shape — but a caller that
// skips it can store a rising decay ladder, which silently removes the
// tightening ceiling.
func UpsertOfflineConfig(
	ctx context.Context, pool *pgxpool.Pool,
	eventID string, c OfflineConfig, updatedBy string,
) error {
	spendCents, err := ToCents(c.MaxOfflineSpend)
	if err != nil {
		return err
	}
	allowCents, err := ToCents(c.UnknownTagAllowance)
	if err != nil {
		return err
	}
	decayJSON, err := json.Marshal(c.DecaySteps)
	if err != nil {
		return err
	}

	policy := "disabled"
	if c.Enabled {
		policy = "enabled"
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO event_cashless_config
			(event_id, offline_policy, max_offline_taps, max_offline_spend_cents,
			 max_distinct_vendors, decay_steps, unknown_tag_allowance_cents,
			 max_recovery_attempts, updated_by)
		VALUES ($1, $2::offline_spend_policy, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (event_id) DO UPDATE SET
			offline_policy              = EXCLUDED.offline_policy,
			max_offline_taps            = EXCLUDED.max_offline_taps,
			max_offline_spend_cents     = EXCLUDED.max_offline_spend_cents,
			max_distinct_vendors        = EXCLUDED.max_distinct_vendors,
			decay_steps                 = EXCLUDED.decay_steps,
			unknown_tag_allowance_cents = EXCLUDED.unknown_tag_allowance_cents,
			max_recovery_attempts       = EXCLUDED.max_recovery_attempts,
			updated_by                  = EXCLUDED.updated_by
	`, eventID, policy, c.MaxOfflineTaps, spendCents, c.MaxDistinctVendors,
		decayJSON, allowCents, c.MaxRecoveryAttempts, updatedBy)
	return err
}

// DisableOfflineSpend is the panic button.
//
// A single statement so it cannot half-apply, and it does three things at once
// because doing them separately leaves windows: the policy goes off, every
// till loses offline mode, and every live key grant is revoked. A device that
// kept a usable event key while the policy said disabled would be the worst of
// both.
//
// Does NOT reach devices already offline — nothing can. Their queues still sync
// and every transaction is flagged by the recheck, which is the honest handling.
func DisableOfflineSpend(ctx context.Context, pool *pgxpool.Pool, eventID, reason string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO event_cashless_config (event_id, offline_policy)
		VALUES ($1, 'disabled')
		ON CONFLICT (event_id) DO UPDATE SET offline_policy = 'disabled'
	`, eventID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE vendor_pos_devices SET offline_enabled = FALSE WHERE event_id = $1
	`, eventID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pos_key_grants SET revoked_at = now(), revoke_reason = $2
		WHERE event_id = $1 AND revoked_at IS NULL
	`, eventID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ── Balance snapshots ────────────────────────────────────────────────────────

// WalletsNeedingSnapshot returns wallets whose ledger has grown well past their
// last checkpoint.
//
// Balance is SUM over the whole ledger, so a wallet with two thousand entries
// re-sums two thousand rows on every tap. Snapshots turn that into a checkpoint
// plus a small delta — the §5 latency concern, and the reason the 3-second
// transaction target survives a busy festival.
//
// Threshold rather than a schedule: a wallet with four transactions gains
// nothing from a snapshot, and writing one for every wallet nightly is work
// nobody needs.
func WalletsNeedingSnapshot(ctx context.Context, pool *pgxpool.Pool, minNewEntries int, limit int) ([]string, error) {
	if minNewEntries <= 0 {
		minNewEntries = 200
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := pool.Query(ctx, `
		SELECT t.wallet_id
		FROM cashless_transactions t
		LEFT JOIN LATERAL (
			SELECT as_of_seq_num FROM wallet_balance_snapshots s
			WHERE s.wallet_id = t.wallet_id
			ORDER BY s.as_of_seq_num DESC LIMIT 1
		) snap ON TRUE
		GROUP BY t.wallet_id, snap.as_of_seq_num
		HAVING MAX(t.seq_num) - COALESCE(snap.as_of_seq_num, 0) >= $1
		LIMIT $2
	`, minNewEntries, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}