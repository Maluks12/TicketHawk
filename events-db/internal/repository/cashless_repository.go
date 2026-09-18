package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"events-db/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Cashless repository ───────────────────────────────────────────────────────
//
// Every money-moving function here follows the same shape, and the shape is the
// safety property:
//
//	BEGIN
//	  SELECT ... FROM cashless_wallets WHERE id = $1 FOR UPDATE   -- serialise
//	  compute balance from the ledger
//	  authorise
//	  INSERT the entry (seq, prev_hash, hash)
//	COMMIT
//
// The row lock is what makes concurrent tills safe. Two bars ringing up the
// same band at the same instant would otherwise both read the old balance and
// both approve, taking the wallet negative — the classic double-spend, and it
// needs no attacker, just a busy Saturday. Locking the wallet row makes the
// second transaction wait for the first, so it reads a balance that already
// includes the first sale.
//
// Balances are summed in POSTGRES over DECIMAL, then converted to int64 cents
// for the comparison. Go never accumulates money in float64 — see the header of
// cashless_ledger.go.

// Only the errors that are actually returned. ErrTopupNotFound in particular is
// absent on purpose: ad_wallet_repository.go already declares that name in this
// package, and the top-up paths here treat an unknown reference as a silent
// no-op rather than an error, so there was nothing for it to do but collide.
var (
	ErrWalletNotFound   = errors.New("wallet not found")
	ErrPOSDeviceInvalid = errors.New("pos device token invalid")
	ErrTagHasNoWallet   = errors.New("tag is not linked to a wallet")
)

// SpendRefusal carries the machine-readable reason alongside the error so the
// handler can map it to an HTTP status and the till can show a cashier
// something they can act on.
type SpendRefusal struct {
	Reason  string
	Balance float64
}

func (e *SpendRefusal) Error() string { return "spend refused: " + e.Reason }

// ── Vendors ──────────────────────────────────────────────────────────────────

func CreateVendor(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, name, code string, commission *float64, createdBy string,
) (*models.EventVendor, error) {
	v := &models.EventVendor{}
	err := pool.QueryRow(ctx, `
		INSERT INTO event_vendors (event_id, name, code, commission_rate, created_by)
		VALUES ($1, $2, upper($3), $4, $5)
		RETURNING id, event_id, name, code, contact_name, contact_email, contact_phone,
		          commission_rate, status::text, created_by, created_at, updated_at
	`, eventID, name, code, commission, createdBy).Scan(
		&v.ID, &v.EventID, &v.Name, &v.Code, &v.ContactName, &v.ContactEmail,
		&v.ContactPhone, &v.CommissionRate, &v.Status, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func ListVendors(ctx context.Context, pool *pgxpool.Pool, eventID string) ([]models.EventVendor, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, event_id, name, code, contact_name, contact_email, contact_phone,
		       commission_rate, status::text, created_by, created_at, updated_at
		FROM event_vendors WHERE event_id = $1 ORDER BY code
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.EventVendor{}
	for rows.Next() {
		var v models.EventVendor
		if err := rows.Scan(&v.ID, &v.EventID, &v.Name, &v.Code, &v.ContactName,
			&v.ContactEmail, &v.ContactPhone, &v.CommissionRate, &v.Status,
			&v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func UpsertVendorProduct(
	ctx context.Context, pool *pgxpool.Pool,
	vendorID, name string, category *string, price float64, sortOrder int,
) (*models.VendorProduct, error) {
	p := &models.VendorProduct{}
	err := pool.QueryRow(ctx, `
		INSERT INTO vendor_products (vendor_id, name, category, price, sort_order)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, vendor_id, name, category, price, is_active, sort_order, created_at, updated_at
	`, vendorID, name, category, price, sortOrder).Scan(
		&p.ID, &p.VendorID, &p.Name, &p.Category, &p.Price, &p.IsActive,
		&p.SortOrder, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func ListVendorProducts(ctx context.Context, pool *pgxpool.Pool, vendorID string) ([]models.VendorProduct, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, vendor_id, name, category, price, is_active, sort_order, created_at, updated_at
		FROM vendor_products WHERE vendor_id = $1 AND is_active ORDER BY sort_order, name
	`, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.VendorProduct{}
	for rows.Next() {
		var p models.VendorProduct
		if err := rows.Scan(&p.ID, &p.VendorID, &p.Name, &p.Category, &p.Price,
			&p.IsActive, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── POS devices ──────────────────────────────────────────────────────────────

// ProvisionPOSDevice issues a till token. The plaintext is returned once and
// never stored — only its SHA-256. A database dump must not hand out working
// till credentials, and a lost token means re-provisioning rather than a lookup.
func ProvisionPOSDevice(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, vendorID, label, createdBy string,
) (*models.VendorPOSDevice, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generating device token: %w", err)
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))

	d := &models.VendorPOSDevice{}
	err := pool.QueryRow(ctx, `
		INSERT INTO vendor_pos_devices (event_id, vendor_id, label, token_hash, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, event_id, vendor_id, label, status::text, last_seen_at, created_at
	`, eventID, vendorID, label, hex.EncodeToString(sum[:]), createdBy).Scan(
		&d.ID, &d.EventID, &d.VendorID, &d.Label, &d.Status, &d.LastSeenAt, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	d.Token = token // the only time this is ever visible
	return d, nil
}

// ResolvePOSDevice authenticates a till by its token and returns which vendor
// it belongs to.
//
// Device identity answers "which till rang this up", which a human credential
// cannot. The operator's own staff authorisation is checked separately by the
// handler, so revoking a person still cuts them off exactly as at the gate.
func ResolvePOSDevice(ctx context.Context, pool *pgxpool.Pool, eventID, token string) (deviceID, vendorID string, err error) {
	sum := sha256.Sum256([]byte(token))
	err = pool.QueryRow(ctx, `
		UPDATE vendor_pos_devices
		SET last_seen_at = now()
		WHERE event_id = $1 AND token_hash = $2 AND status = 'active'
		RETURNING id, vendor_id
	`, eventID, hex.EncodeToString(sum[:])).Scan(&deviceID, &vendorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrPOSDeviceInvalid
	}
	return deviceID, vendorID, err
}

// ── Wallets ──────────────────────────────────────────────────────────────────

// GetOrCreateWallet returns the attendee's wallet for an event.
//
// userID may be nil: a cash walk-up at a top-up booth gets an anonymous wallet.
// Requiring an account would exclude exactly the attendees a cashless event most
// risks excluding, and uq_wallet_event_user is partial so many anonymous
// wallets coexist happily.
func GetOrCreateWallet(ctx context.Context, pool *pgxpool.Pool, eventID string, userID *string) (*models.CashlessWallet, error) {
	if userID != nil {
		w, err := getWalletByUser(ctx, pool, eventID, *userID)
		if err == nil {
			return w, nil
		}
		if !errors.Is(err, ErrWalletNotFound) {
			return nil, err
		}
	}

	w := &models.CashlessWallet{}
	err := pool.QueryRow(ctx, `
		INSERT INTO cashless_wallets (event_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (event_id, user_id) DO UPDATE SET updated_at = now()
		RETURNING id, event_id, user_id, status::text, currency, funding::text, card_on_file, created_at
	`, eventID, userID).Scan(&w.ID, &w.EventID, &w.UserID, &w.Status, &w.Currency,
		&w.Funding, &w.CardOnFile, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func getWalletByUser(ctx context.Context, pool *pgxpool.Pool, eventID, userID string) (*models.CashlessWallet, error) {
	w := &models.CashlessWallet{}
	err := pool.QueryRow(ctx, `
		SELECT w.id, w.event_id, w.user_id, w.status::text, w.currency,
		       w.funding::text, w.card_on_file, w.created_at,
		       COALESCE((SELECT SUM(amount) FROM cashless_transactions t WHERE t.wallet_id = w.id), 0)
		FROM cashless_wallets w
		WHERE w.event_id = $1 AND w.user_id = $2
	`, eventID, userID).Scan(&w.ID, &w.EventID, &w.UserID, &w.Status, &w.Currency,
		&w.Funding, &w.CardOnFile, &w.CreatedAt, &w.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWalletNotFound
	}
	return w, err
}

// GetWallet reads one wallet with its computed balance.
func GetWallet(ctx context.Context, pool *pgxpool.Pool, walletID string) (*models.CashlessWallet, error) {
	w := &models.CashlessWallet{}
	err := pool.QueryRow(ctx, `
		SELECT w.id, w.event_id, w.user_id, w.status::text, w.currency,
		       w.funding::text, w.card_on_file, w.created_at,
		       COALESCE((SELECT SUM(amount) FROM cashless_transactions t WHERE t.wallet_id = w.id), 0)
		FROM cashless_wallets w WHERE w.id = $1
	`, walletID).Scan(&w.ID, &w.EventID, &w.UserID, &w.Status, &w.Currency,
		&w.Funding, &w.CardOnFile, &w.CreatedAt, &w.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWalletNotFound
	}
	return w, err
}

// BindWalletToTag links a band to a wallet.
//
// chk_rfid_wallet_requires_verifiable_chip refuses a uid_only chip in the
// database; the explicit check here exists to turn that constraint violation
// into a sentence an operator at a desk can act on.
func BindWalletToTag(ctx context.Context, pool *pgxpool.Pool, eventID, tagUID, walletID string) error {
	var class, status string
	err := pool.QueryRow(ctx, `
		SELECT verification_class::text, status::text
		FROM rfid_tags WHERE event_id = $1 AND tag_uid = $2
	`, eventID, tagUID).Scan(&class, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTagNotFound
	}
	if err != nil {
		return err
	}
	if class == "unverified" {
		return fmt.Errorf("this band's chip type cannot hold a wallet — it is cloneable")
	}

	res, err := pool.Exec(ctx, `
		UPDATE rfid_tags SET wallet_id = $3
		WHERE event_id = $1 AND tag_uid = $2 AND status IN ('provisioned','encoded','active')
	`, eventID, tagUID, walletID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrTagAlreadyBound
	}
	return nil
}

// ── The spend path ───────────────────────────────────────────────────────────

// TagSpendTarget is a band resolved for spending.
type TagSpendTarget struct {
	TagUID   string
	TagID    string
	WalletID *string
	Context  SpendContext
}

// ResolveTagForSpend loads everything AuthorizeSpend needs, in one round trip.
func ResolveTagForSpend(ctx context.Context, pool *pgxpool.Pool, eventID, tagUID string) (*TagSpendTarget, error) {
	t := &TagSpendTarget{TagUID: tagUID}
	var balance float64
	err := pool.QueryRow(ctx, `
		SELECT rt.id, rt.status::text, rt.verification_class::text, rt.wallet_id,
		       COALESCE(w.status::text, ''),
		       COALESCE((SELECT SUM(amount) FROM cashless_transactions t WHERE t.wallet_id = rt.wallet_id), 0)
		FROM rfid_tags rt
		LEFT JOIN cashless_wallets w ON w.id = rt.wallet_id
		WHERE rt.event_id = $1 AND rt.tag_uid = $2
	`, eventID, tagUID).Scan(&t.TagID, &t.Context.TagStatus, &t.Context.VerificationClass,
		&t.WalletID, &t.Context.WalletStatus, &balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTagNotFound
	}
	if err != nil {
		return nil, err
	}

	cents, err := ToCents(balance)
	if err != nil {
		return nil, fmt.Errorf("wallet balance out of range: %w", err)
	}
	t.Context.WalletID = t.WalletID
	t.Context.BalanceCents = cents
	return t, nil
}

// RecordPurchase writes one sale against a band's wallet.
//
// txnID is CLIENT-generated, exactly like scan_events.id: a till that loses its
// response and retries must not charge twice, and letting the client name the
// transaction is the only reliable way to guarantee that. A repeat of a known
// id returns the original outcome and writes nothing.
//
// The wallet row is locked FOR UPDATE before the balance is read, so two tills
// ringing up the same band at the same instant serialise instead of both
// approving against a stale balance.
func RecordPurchase(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, txnID, tagUID, vendorID, deviceID string,
	amountCents int64, detail map[string]any, createdBy string,
) (*models.CashlessTransaction, error) {
	if detail == nil {
		detail = map[string]any{}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Idempotency first, before any locking: a retry must be cheap and must not
	// queue behind a busy wallet.
	if existing, err := getTxnTx(ctx, tx, txnID); err == nil {
		return existing, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var (
		tagStatus, class, walletStatus string
		walletID                       *string
	)
	err = tx.QueryRow(ctx, `
		SELECT rt.status::text, rt.verification_class::text, rt.wallet_id,
		       COALESCE(w.status::text, '')
		FROM rfid_tags rt
		LEFT JOIN cashless_wallets w ON w.id = rt.wallet_id
		WHERE rt.event_id = $1 AND rt.tag_uid = $2
	`, eventID, tagUID).Scan(&tagStatus, &class, &walletID, &walletStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTagNotFound
	}
	if err != nil {
		return nil, err
	}
	if walletID == nil {
		return nil, ErrTagHasNoWallet
	}

	// Lock the wallet, THEN read the balance. Reversing these two lines
	// reintroduces the double-spend this whole function exists to prevent.
	var lockedStatus string
	if err := tx.QueryRow(ctx,
		`SELECT status::text FROM cashless_wallets WHERE id = $1 FOR UPDATE`,
		*walletID).Scan(&lockedStatus); err != nil {
		return nil, err
	}

	var balance float64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM cashless_transactions WHERE wallet_id = $1`,
		*walletID).Scan(&balance); err != nil {
		return nil, err
	}
	balanceCents, err := ToCents(balance)
	if err != nil {
		return nil, err
	}

	if reason := AuthorizeSpend(SpendContext{
		TagStatus:         tagStatus,
		VerificationClass: class,
		WalletID:          walletID,
		WalletStatus:      lockedStatus,
		BalanceCents:      balanceCents,
	}, amountCents); reason != SpendOK {
		return nil, &SpendRefusal{Reason: reason, Balance: balance}
	}

	signed, err := SignedCents(TxnPurchase, amountCents)
	if err != nil {
		return nil, err
	}

	txn, err := appendEntry(ctx, tx, entryInput{
		ID: txnID, WalletID: *walletID, EventID: eventID,
		Kind: TxnPurchase, SignedCents: signed,
		VendorID: &vendorID, DeviceID: &deviceID, TagUID: &tagUID,
		Detail: detail, CreatedBy: &createdBy,
	})
	if err != nil {
		return nil, err
	}
	return txn, tx.Commit(ctx)
}

// ── Ledger append ────────────────────────────────────────────────────────────

type entryInput struct {
	ID          string
	WalletID    string
	EventID     string
	Kind        string
	SignedCents int64
	VendorID    *string
	DeviceID    *string
	TagUID      *string
	Detail      map[string]any
	CreatedBy   *string
}

// appendEntry writes one ledger row, computing its sequence number and hash
// link from the wallet's current head.
//
// Callers MUST already hold the wallet's FOR UPDATE lock. Without it two
// concurrent appends read the same head and race on uq_cashless_wallet_seq —
// the unique constraint catches it, so nothing corrupts, but one till gets an
// opaque error instead of a sale.
func appendEntry(ctx context.Context, tx pgx.Tx, in entryInput) (*models.CashlessTransaction, error) {
	var (
		seq      int64
		prevHash *string
	)
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(seq_num), 0), (
			SELECT hash FROM cashless_transactions
			WHERE wallet_id = $1 ORDER BY seq_num DESC LIMIT 1
		)
		FROM cashless_transactions WHERE wallet_id = $1
	`, in.WalletID).Scan(&seq, &prevHash)
	if err != nil {
		return nil, err
	}
	seq++

	prev := ""
	if prevHash != nil {
		prev = *prevHash
	}
	vendor, tag := "", ""
	if in.VendorID != nil {
		vendor = *in.VendorID
	}
	if in.TagUID != nil {
		tag = *in.TagUID
	}
	hash := ChainHash(prev, in.WalletID, in.Kind, seq, in.SignedCents, vendor, tag)

	t := &models.CashlessTransaction{}
	err = tx.QueryRow(ctx, `
		INSERT INTO cashless_transactions
			(id, wallet_id, event_id, kind, amount, vendor_id, device_id, tag_uid,
			 seq_num, prev_hash, hash, mode, detail, created_by)
		VALUES ($1, $2, $3, $4::cashless_txn_kind, $5, $6, $7, $8, $9, $10, $11, 'online', $12, $13)
		RETURNING id, wallet_id, kind::text, amount, vendor_id, tag_uid, seq_num, mode::text, detail, created_at
	`,
		in.ID, in.WalletID, in.EventID, in.Kind, FromCents(in.SignedCents),
		in.VendorID, in.DeviceID, in.TagUID, seq, nullIfEmpty(prev), hash,
		in.Detail, in.CreatedBy,
	).Scan(&t.ID, &t.WalletID, &t.Kind, &t.Amount, &t.VendorID, &t.TagUID,
		&t.SeqNum, &t.Mode, &t.Detail, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func getTxnTx(ctx context.Context, tx pgx.Tx, txnID string) (*models.CashlessTransaction, error) {
	t := &models.CashlessTransaction{}
	err := tx.QueryRow(ctx, `
		SELECT id, wallet_id, kind::text, amount, vendor_id, tag_uid, seq_num, mode::text, detail, created_at
		FROM cashless_transactions WHERE id = $1
	`, txnID).Scan(&t.ID, &t.WalletID, &t.Kind, &t.Amount, &t.VendorID, &t.TagUID,
		&t.SeqNum, &t.Mode, &t.Detail, &t.CreatedAt)
	return t, err
}

// ── Top-ups ──────────────────────────────────────────────────────────────────

// CreateCashTopup takes notes at a booth and credits immediately.
//
// Cash has no gateway to confirm it, so the ledger entry is written in the same
// transaction as the topup row. The staff member is recorded because that
// attribution is the only audit trail cash has.
func CreateCashTopup(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, walletID string, amountCents, feeCents int64, staffID string,
) (*models.CashlessTransaction, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status::text FROM cashless_wallets WHERE id = $1 FOR UPDATE`,
		walletID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWalletNotFound
	} else if err != nil {
		return nil, err
	}
	if status != WalletActive {
		return nil, &SpendRefusal{Reason: SpendWalletNotActive}
	}

	txnID := uuid.NewString()
	var topupID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO cashless_topups
			(wallet_id, event_id, amount, fee, method, status, created_by, completed_at)
		VALUES ($1, $2, $3, $4, 'cash', 'completed', $5, now())
		RETURNING id
	`, walletID, eventID, FromCents(amountCents), FromCents(feeCents), staffID).Scan(&topupID); err != nil {
		return nil, err
	}

	// The credited amount is the money taken MINUS the disclosed fee (§6:
	// "$50 top-up, $2 service fee, $48 credited"). The fee is stored on the
	// topup row so a statement can show exactly what the attendee was told,
	// even after the event's fee changes.
	credited, err := SignedCents(TxnTopup, amountCents-feeCents)
	if err != nil {
		return nil, err
	}

	txn, err := appendEntry(ctx, tx, entryInput{
		ID: txnID, WalletID: walletID, EventID: eventID,
		Kind: TxnTopup, SignedCents: credited,
		Detail:    map[string]any{"method": "cash", "gross": FromCents(amountCents), "fee": FromCents(feeCents)},
		CreatedBy: &staffID,
	})
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE cashless_topups SET txn_id = $2 WHERE id = $1`, topupID, txnID); err != nil {
		return nil, err
	}
	return txn, tx.Commit(ctx)
}

// CreatePendingCardTopup records the intent before the attendee is sent to
// Paystack. Nothing is credited until the webhook confirms.
func CreatePendingCardTopup(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, walletID string, amountCents, feeCents int64, reference string,
) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO cashless_topups
			(wallet_id, event_id, amount, fee, method, status, paystack_reference)
		VALUES ($1, $2, $3, $4, 'card', 'pending', $5)
	`, walletID, eventID, FromCents(amountCents), FromCents(feeCents), reference)
	return err
}

// ConfirmCardTopup credits a wallet once Paystack confirms the charge.
//
// Mirrors ConfirmAdWalletTopup: an unknown or already-processed reference is a
// silent no-op so a duplicate webhook delivery still 200s. Paystack retries,
// and a second delivery must not credit twice.
func ConfirmCardTopup(ctx context.Context, pool *pgxpool.Pool, reference string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		topupID, walletID, eventID string
		amount, fee                float64
	)
	err = tx.QueryRow(ctx, `
		UPDATE cashless_topups
		SET status = 'completed', completed_at = now()
		WHERE paystack_reference = $1 AND status = 'pending'
		RETURNING id, wallet_id, event_id, amount, fee
	`, reference).Scan(&topupID, &walletID, &eventID, &amount, &fee)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // duplicate delivery or unknown reference
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`SELECT id FROM cashless_wallets WHERE id = $1 FOR UPDATE`, walletID); err != nil {
		return err
	}

	amountCents, err := ToCents(amount)
	if err != nil {
		return err
	}
	feeCents, err := ToCents(fee)
	if err != nil {
		return err
	}
	credited, err := SignedCents(TxnTopup, amountCents-feeCents)
	if err != nil {
		return err
	}

	txnID := uuid.NewString()
	if _, err := appendEntry(ctx, tx, entryInput{
		ID: txnID, WalletID: walletID, EventID: eventID,
		Kind: TxnTopup, SignedCents: credited,
		Detail: map[string]any{"method": "card", "gross": amount, "fee": fee, "reference": reference},
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE cashless_topups SET txn_id = $2 WHERE id = $1`, topupID, txnID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FailCardTopup marks an abandoned or declined card top-up. No ledger entry is
// written, because no money moved.
func FailCardTopup(ctx context.Context, pool *pgxpool.Pool, reference string) error {
	_, err := pool.Exec(ctx, `
		UPDATE cashless_topups SET status = 'failed'
		WHERE paystack_reference = $1 AND status = 'pending'
	`, reference)
	return err
}

// ── Statements ───────────────────────────────────────────────────────────────

// ListWalletTransactions returns a wallet's statement, newest first.
func ListWalletTransactions(ctx context.Context, pool *pgxpool.Pool, walletID string, limit int) ([]models.CashlessTransaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := pool.Query(ctx, `
		SELECT t.id, t.wallet_id, t.kind::text, t.amount, t.vendor_id, v.name,
		       t.tag_uid, t.seq_num, t.mode::text, t.detail, t.created_at
		FROM cashless_transactions t
		LEFT JOIN event_vendors v ON v.id = t.vendor_id
		WHERE t.wallet_id = $1
		ORDER BY t.seq_num DESC
		LIMIT $2
	`, walletID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.CashlessTransaction{}
	for rows.Next() {
		var t models.CashlessTransaction
		if err := rows.Scan(&t.ID, &t.WalletID, &t.Kind, &t.Amount, &t.VendorID,
			&t.VendorName, &t.TagUID, &t.SeqNum, &t.Mode, &t.Detail, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// VerifyWalletChain re-walks a wallet's hash chain and reports the first broken
// sequence number, or 0 when intact.
//
// Reconciliation only — O(history), never on the spend path.
func VerifyWalletChain(ctx context.Context, pool *pgxpool.Pool, walletID string) (int64, error) {
	rows, err := pool.Query(ctx, `
		SELECT wallet_id, kind::text, seq_num, amount, COALESCE(vendor_id::text, ''),
		       COALESCE(tag_uid, ''), COALESCE(prev_hash, ''), hash
		FROM cashless_transactions WHERE wallet_id = $1 ORDER BY seq_num
	`, walletID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	entries := []ChainEntry{}
	seqs := []int64{}
	for rows.Next() {
		var e ChainEntry
		var amount float64
		if err := rows.Scan(&e.WalletID, &e.Kind, &e.Seq, &amount,
			&e.VendorID, &e.TagUID, &e.PrevHash, &e.Hash); err != nil {
			return 0, err
		}
		if e.SignedCents, err = ToCents(amount); err != nil {
			return 0, err
		}
		entries = append(entries, e)
		seqs = append(seqs, e.Seq)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if idx := VerifyChain(entries); idx >= 0 {
		return seqs[idx], nil
	}
	return 0, nil
}

// ── Snapshots ────────────────────────────────────────────────────────────────

// WriteBalanceSnapshot checkpoints a wallet so balance reads become
// snapshot-plus-delta rather than a full re-sum. A read aid only: delete every
// snapshot and nothing is lost but speed.
func WriteBalanceSnapshot(ctx context.Context, pool *pgxpool.Pool, walletID string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO wallet_balance_snapshots (wallet_id, snapshot_seq, balance, as_of_seq_num)
		SELECT $1,
		       COALESCE((SELECT MAX(snapshot_seq) FROM wallet_balance_snapshots WHERE wallet_id = $1), 0) + 1,
		       COALESCE(SUM(amount), 0),
		       COALESCE(MAX(seq_num), 0)
		FROM cashless_transactions WHERE wallet_id = $1
		ON CONFLICT (wallet_id, snapshot_seq) DO NOTHING
	`, walletID)
	return err
}