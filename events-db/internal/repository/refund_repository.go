package repository

import (
	"context"
	"errors"

	"events-db/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Refunds and adjustments ───────────────────────────────────────────────────
//
// Same transactional shape as RecordPurchase: lock the wallet, read the ledger,
// authorise, append. A refund credits rather than debits, but it still competes
// with concurrent purchases for the same wallet, and skipping the lock would
// let a refund and a sale interleave into a balance neither expected.

var ErrRefundRefused = errors.New("refund refused")

// RefundRefusal carries the machine-readable reason and what remains
// refundable, so a till can offer the cashier a partial rather than a flat no.
type RefundRefusal struct {
	Reason           string
	RemainingRefund  float64
	OriginalAmount   float64
}

func (e *RefundRefusal) Error() string { return "refund refused: " + e.Reason }

// RecordRefund reverses part or all of a sale.
//
// txnID is CLIENT-generated, like every other money-moving path: a till that
// loses its response and retries must not refund twice, and letting the client
// name the transaction is the only reliable guarantee of that. The stakes are
// higher here than for a purchase — a duplicate refund pays an attendee twice
// out of the vendor's settlement.
func RecordRefund(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, txnID, originalTxnID, requestingVendorID, deviceID string,
	amountCents int64, reason, createdBy string,
) (*models.CashlessTransaction, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Idempotency first, before any locking — a retry must be cheap and must
	// not queue behind a busy wallet.
	if existing, err := getTxnTx(ctx, tx, txnID); err == nil {
		return existing, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// Resolve the original sale, how much has already gone back against it, and
	// the wallet's state — in one round trip, so the picture is consistent.
	var (
		rc       RefundContext
		walletID string
		origAmt  float64
		refunded float64
		vendorID *string
		tagUID   *string
	)
	err = tx.QueryRow(ctx, `
		SELECT t.wallet_id, t.kind::text, t.amount, t.vendor_id, t.tag_uid,
		       COALESCE(w.status::text, ''),
		       COALESCE((SELECT SUM(r.amount) FROM cashless_transactions r
		                  WHERE r.refunds_txn_id = t.id), 0)
		FROM cashless_transactions t
		JOIN cashless_wallets w ON w.id = t.wallet_id
		WHERE t.id = $1 AND t.event_id = $2
	`, originalTxnID, eventID).Scan(&walletID, &rc.OriginalKind, &origAmt,
		&vendorID, &tagUID, &rc.WalletStatus, &refunded)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, &RefundRefusal{Reason: RefundNoOriginal}
	case err != nil:
		return nil, err
	}

	rc.Found = true
	if vendorID != nil {
		rc.VendorID = *vendorID
	}
	// A purchase is stored NEGATIVE; the refund logic works in positive
	// magnitudes, so flip it here rather than making every caller remember.
	if c, err := ToCents(-origAmt); err == nil {
		rc.OriginalCents = c
	}
	if c, err := ToCents(refunded); err == nil {
		rc.AlreadyRefundedCents = c
	}

	if reasonCode := AuthorizeRefund(rc, requestingVendorID, amountCents, reason); reasonCode != RefundOK {
		return nil, &RefundRefusal{
			Reason:          reasonCode,
			RemainingRefund: FromCents(RemainingRefundableCents(rc)),
			OriginalAmount:  FromCents(rc.OriginalCents),
		}
	}

	// Lock, then append. Same ordering discipline as RecordPurchase.
	if _, err := tx.Exec(ctx,
		`SELECT id FROM cashless_wallets WHERE id = $1 FOR UPDATE`, walletID); err != nil {
		return nil, err
	}

	signed, err := SignedCents(TxnRefund, amountCents)
	if err != nil {
		return nil, err
	}

	txn, err := appendEntry(ctx, tx, entryInput{
		ID: txnID, WalletID: walletID, EventID: eventID,
		Kind: TxnRefund, SignedCents: signed,
		VendorID: &requestingVendorID, DeviceID: &deviceID, TagUID: tagUID,
		Detail:    map[string]any{"refunds": originalTxnID},
		CreatedBy: &createdBy,
	})
	if err != nil {
		return nil, err
	}

	// The link and the reason go on in a second statement rather than widening
	// appendEntry, which every online call site would then thread nils through
	// for no benefit. Unambiguous because the row was just inserted by a known
	// id — unlike the raw_ref re-match this pattern replaced in Phase 2.
	if _, err := tx.Exec(ctx, `
		UPDATE cashless_transactions SET refunds_txn_id = $2, reason = $3 WHERE id = $1
	`, txnID, originalTxnID, reason); err != nil {
		return nil, err
	}

	return txn, tx.Commit(ctx)
}

// RecordAdjustment applies a manual correction.
//
// signedCents carries its own direction — positive credits, negative debits.
// The only path where the caller chooses the sign, because a correction has to
// be able to undo whatever went wrong.
func RecordAdjustment(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, walletID string, signedCents int64, reason, createdBy string,
) (*models.CashlessTransaction, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status::text FROM cashless_wallets WHERE id = $1 AND event_id = $2 FOR UPDATE`,
		walletID, eventID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWalletNotFound
	} else if err != nil {
		return nil, err
	}

	var balance float64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM cashless_transactions WHERE wallet_id = $1`,
		walletID).Scan(&balance); err != nil {
		return nil, err
	}
	balanceCents, err := ToCents(balance)
	if err != nil {
		return nil, err
	}

	if reasonCode := AuthorizeAdjustment(status, balanceCents, signedCents, reason); reasonCode != AdjustmentOK {
		return nil, &SpendRefusal{Reason: reasonCode, Balance: balance}
	}

	txn, err := appendEntry(ctx, tx, entryInput{
		ID: uuid.NewString(), WalletID: walletID, EventID: eventID,
		Kind: TxnAdjustment, SignedCents: signedCents,
		Detail:    map[string]any{"manual": true},
		CreatedBy: &createdBy,
	})
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE cashless_transactions SET reason = $2 WHERE id = $1`, txn.ID, reason); err != nil {
		return nil, err
	}
	return txn, tx.Commit(ctx)
}

// GetRefundableTransaction resolves a sale for the till's confirm-before-refund
// screen, so a cashier sees what remains before committing.
func GetRefundableTransaction(
	ctx context.Context, pool *pgxpool.Pool, eventID, txnID string,
) (*RefundContext, float64, error) {
	var (
		rc       RefundContext
		origAmt  float64
		refunded float64
		vendorID *string
	)
	err := pool.QueryRow(ctx, `
		SELECT t.kind::text, t.amount, t.vendor_id, COALESCE(w.status::text, ''),
		       COALESCE((SELECT SUM(r.amount) FROM cashless_transactions r
		                  WHERE r.refunds_txn_id = t.id), 0)
		FROM cashless_transactions t
		JOIN cashless_wallets w ON w.id = t.wallet_id
		WHERE t.id = $1 AND t.event_id = $2
	`, txnID, eventID).Scan(&rc.OriginalKind, &origAmt, &vendorID, &rc.WalletStatus, &refunded)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrTxnNotFound
	}
	if err != nil {
		return nil, 0, err
	}

	rc.Found = true
	if vendorID != nil {
		rc.VendorID = *vendorID
	}
	if c, err := ToCents(-origAmt); err == nil {
		rc.OriginalCents = c
	}
	if c, err := ToCents(refunded); err == nil {
		rc.AlreadyRefundedCents = c
	}
	return &rc, FromCents(RemainingRefundableCents(rc)), nil
}

var ErrTxnNotFound = errors.New("transaction not found")