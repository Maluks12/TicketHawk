package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Offline transaction ingestion ─────────────────────────────────────────────
//
// Mirrors IngestScans, deliberately: client-generated ids, per-item outcomes,
// server-decided verdicts, and a batch that keeps going when one item is bad.
// The scan path proved that shape works through real outages, and money is not
// a reason to invent a second one.
//
// WHAT THE SERVER DOES THAT THE DEVICE COULD NOT:
//
//  1. Re-runs the SAME cap logic (RecheckOfflineApproval) against what it knows
//     now. A disagreement means the till approved something the rules forbid.
//  2. Verifies the device signature, so a transaction cannot be forged in a
//     till's name and a till cannot disown one it signed.
//  3. Enforces the per-device sequence, so a vendor cannot drop or reorder
//     entries in their own queue to hide a sale.
//  4. Applies the ledger, which is the only place a balance actually moves.
//
// WHAT IT DOES NOT DO: reverse a settled sale on a disagreement. The attendee
// already has their drink, and clawing back on a recomputation punishes them
// for a vendor's device. Anomalies are FLAGGED — same philosophy as accepting
// an unsigned transaction rather than discarding the evidence.

// OfflineTxn is one queued sale as the till reports it.
type OfflineTxn struct {
	ID           string    `json:"id" binding:"required,uuid"` // client-generated
	TagUID       string    `json:"tag_uid" binding:"required"`
	AmountCents  int64     `json:"amount_cents" binding:"required,gt=0"`
	DeviceSeq    int64     `json:"device_seq" binding:"required,gt=0"`
	AccumBefore  int64     `json:"accum_before"`
	AccumAfter   int64     `json:"accum_after"`
	ApprovedAt   time.Time `json:"approved_at" binding:"required"`
	Signature    string    `json:"signature"`
	Items        []map[string]any `json:"items,omitempty"`

	// What the DEVICE believed when it approved. Carried so the server can
	// re-run the caps against the device's own view and tell "the till followed
	// the rules with stale data" apart from "the till ignored the rules" —
	// which are very different findings.
	CachedBalanceCents int64 `json:"cached_balance_cents"`
	OfflineTapsAtTime  int   `json:"offline_taps_at_time"`
	OfflineSpentAtTime int64 `json:"offline_spent_at_time"`
	MinutesOffline     int   `json:"minutes_offline"`
	DistinctVendors    int   `json:"distinct_vendors"`
}

// OfflineTxnOutcome is the per-item result.
type OfflineTxnOutcome struct {
	ID       string  `json:"id"`
	Verdict  string  `json:"verdict"` // applied | duplicate | flagged | rejected
	Reason   string  `json:"reason,omitempty"`
	Balance  float64 `json:"balance,omitempty"`
	Flagged  bool    `json:"flagged,omitempty"`
}

// Ingestion verdicts.
const (
	OfflineApplied   = "applied"
	OfflineDuplicate = "duplicate"
	OfflineFlagged   = "flagged"  // applied, but something is wrong — review it
	OfflineRejected  = "rejected" // not applied, no money moved
)

// OfflineSigInput is the flat set of fields a device signs.
//
// Mirrors services.OfflineTxnPayload field for field, and is a separate type
// only because repository cannot import services — services already imports
// repository, and Go rejects the cycle. The handler adapts between them, which
// is the correct side of that constraint: the data layer stays ignorant of who
// consumes it.
type OfflineSigInput struct {
	TxnID, EventID, TagUID, VendorID, DeviceID string
	AmountCents, DeviceSeq                     int64
	AccumBefore, AccumAfter, ApprovedAtMs      int64
}

// SignatureVerifier is the seam to services.VerifyOfflineTxnSignature.
type SignatureVerifier func(pubKeyB64, sigB64 string, in OfflineSigInput) error

// IngestOfflineTxns applies a till's queued sales.
func IngestOfflineTxns(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, deviceID, vendorID string,
	policy OfflinePolicy,
	txns []OfflineTxn,
	verify SignatureVerifier,
) ([]OfflineTxnOutcome, error) {
	out := make([]OfflineTxnOutcome, 0, len(txns))
	for _, t := range txns {
		o, err := ingestOneOfflineTxn(ctx, pool, eventID, deviceID, vendorID, policy, t, verify)
		if err != nil {
			// A hard failure aborts the batch, as IngestScans does: the device
			// keeps its queue and retries, and client-generated ids make that
			// safe. Better than a partial apply nobody can reason about.
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

func ingestOneOfflineTxn(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, deviceID, vendorID string,
	policy OfflinePolicy,
	t OfflineTxn,
	verify SignatureVerifier,
) (*OfflineTxnOutcome, error) {
	res := &OfflineTxnOutcome{ID: t.ID}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// ── Idempotency, before anything expensive ───────────────────────────────
	// A till that lost its response and retried must not charge twice.
	if existing, err := getTxnTx(ctx, tx, t.ID); err == nil {
		res.Verdict = OfflineDuplicate
		res.Balance = existing.Amount
		return res, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// ── Device sequence ──────────────────────────────────────────────────────
	// uq_cashless_device_seq enforces uniqueness, but a GAP is the interesting
	// case: it means entries were dropped from the queue, which is what a
	// vendor hiding a sale looks like. Recorded, not refused — the sale in hand
	// is not the one that went missing.
	var lastSeq int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(device_seq), 0) FROM cashless_transactions WHERE device_id = $1
	`, deviceID).Scan(&lastSeq); err != nil {
		return nil, err
	}
	seqGap := t.DeviceSeq > lastSeq+1

	// ── Signature ────────────────────────────────────────────────────────────
	var pubKey *string
	if err := tx.QueryRow(ctx,
		`SELECT signing_pubkey FROM vendor_pos_devices WHERE id = $1`, deviceID).Scan(&pubKey); err != nil {
		return nil, err
	}
	sigOK := true
	sigReason := ""
	if pubKey == nil || *pubKey == "" {
		sigOK, sigReason = false, "device has no signing key on file"
	} else if err := verify(*pubKey, t.Signature, OfflineSigInput{
		TxnID: t.ID, EventID: eventID, TagUID: t.TagUID,
		VendorID: vendorID, DeviceID: deviceID,
		AmountCents: t.AmountCents, DeviceSeq: t.DeviceSeq,
		AccumBefore: t.AccumBefore, AccumAfter: t.AccumAfter,
		ApprovedAtMs: t.ApprovedAt.UnixMilli(),
	}); err != nil {
		sigOK, sigReason = false, err.Error()
	}

	// ── Resolve the band and wallet ──────────────────────────────────────────
	var (
		tagStatus, class, walletStatus string
		walletID                       *string
		cardOnFile                     bool
	)
	err = tx.QueryRow(ctx, `
		SELECT rt.status::text, rt.verification_class::text, rt.wallet_id,
		       COALESCE(w.status::text, ''), COALESCE(w.card_on_file, FALSE)
		FROM rfid_tags rt
		LEFT JOIN cashless_wallets w ON w.id = rt.wallet_id
		WHERE rt.event_id = $1 AND rt.tag_uid = $2
	`, eventID, t.TagUID).Scan(&tagStatus, &class, &walletID, &walletStatus, &cardOnFile)

	tagKnown := true
	if errors.Is(err, pgx.ErrNoRows) {
		tagKnown = false
	} else if err != nil {
		return nil, err
	}

	if !tagKnown || walletID == nil {
		// Nothing to debit. The sale still happened in the real world, so it is
		// logged as a refusal rather than discarded — an unknown band spending
		// at four stalls is exactly the pattern worth seeing later.
		if err := recordRefusalTx(ctx, tx, t.ID, eventID, deviceID, vendorID,
			t.TagUID, t.AmountCents, "no_wallet_for_band", t.ApprovedAt); err != nil {
			return nil, err
		}
		res.Verdict, res.Reason = OfflineRejected, "band has no wallet"
		return res, tx.Commit(ctx)
	}

	// ── Re-run the caps the device should have applied ───────────────────────
	// Same function the till ran, against the device's OWN reported state. A
	// disagreement here means the till approved something the rules forbid,
	// not that it had stale data.
	recheck := RecheckOfflineApproval(policy,
		OfflineDeviceState{
			Attested: true, OfflineEnabled: true,
			OfflineTaps:       t.OfflineTapsAtTime,
			OfflineSpentCents: t.OfflineSpentAtTime,
			MinutesSinceSync:  t.MinutesOffline,
		},
		OfflineTagState{
			Known: tagKnown, TagStatus: tagStatus, VerificationClass: class,
			CachedBalanceCents:   t.CachedBalanceCents,
			AccumulatorCents:     t.AccumBefore,
			DistinctVendorsSoFar: t.DistinctVendors,
			CardOnFile:           cardOnFile,
		}, t.AmountCents)

	// ── Apply to the ledger ──────────────────────────────────────────────────
	// The wallet lock, then the balance, exactly as RecordPurchase does.
	if _, err := tx.Exec(ctx,
		`SELECT id FROM cashless_wallets WHERE id = $1 FOR UPDATE`, *walletID); err != nil {
		return nil, err
	}

	signed, err := SignedCents(TxnPurchase, t.AmountCents)
	if err != nil {
		return nil, err
	}

	// NOTE: no balance check here, and that is deliberate. Online, insufficient
	// funds refuses. Offline, the money is already spent — refusing now would
	// not un-pour the drink, it would only hide the shortfall from
	// reconciliation. The wallet is allowed to go negative and the recovery
	// ladder picks it up.
	txn, err := appendEntryOffline(ctx, tx, entryInput{
		ID: t.ID, WalletID: *walletID, EventID: eventID,
		Kind: TxnPurchase, SignedCents: signed,
		VendorID: &vendorID, DeviceID: &deviceID, TagUID: &t.TagUID,
		Detail: map[string]any{
			"items":          t.Items,
			"recheck":        recheck,
			"signature_ok":   sigOK,
			"seq_gap":        seqGap,
			"minutes_offline": t.MinutesOffline,
		},
	}, t, sigOK)
	if err != nil {
		return nil, err
	}

	// ── Verdict ──────────────────────────────────────────────────────────────
	flags := []string{}
	if !sigOK {
		flags = append(flags, "signature:"+sigReason)
	}
	if recheck != SpendOK {
		flags = append(flags, "cap_violation:"+recheck)
	}
	if seqGap {
		flags = append(flags, "device_seq_gap")
	}

	res.Verdict = OfflineApplied
	if len(flags) > 0 {
		res.Verdict, res.Flagged = OfflineFlagged, true
		res.Reason = strings.Join(flags, "; ")
	}
	res.Balance = txn.Amount

	return res, tx.Commit(ctx)
}

// appendEntryOffline wraps appendEntry with the offline-specific columns.
func appendEntryOffline(
	ctx context.Context, tx pgx.Tx, in entryInput, t OfflineTxn, sigOK bool,
) (*models.CashlessTransaction, error) {
	txn, err := appendEntry(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	// Stamped separately rather than widening appendEntry's signature, which
	// every online call site would then have to thread nils through for no
	// benefit. The row was just inserted by a known id, so this is unambiguous
	// — unlike the raw_ref re-match this replaced in Phase 2.
	var sig *string
	if t.Signature != "" {
		s := t.Signature
		sig = &s
	}
	_, err = tx.Exec(ctx, `
		UPDATE cashless_transactions
		SET mode = 'offline', device_signature = $2, device_seq = $3,
		    accum_before_cents = $4, accum_after_cents = $5, approved_at = $6
		WHERE id = $1
	`, in.ID, sig, t.DeviceSeq, t.AccumBefore, t.AccumAfter, t.ApprovedAt)
	return txn, err
}

// recordRefusalTx logs an offline sale that could not be applied.
func recordRefusalTx(
	ctx context.Context, tx pgx.Tx,
	id, eventID, deviceID, vendorID, tagUID string,
	amountCents int64, reason string, at time.Time,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO offline_spend_refusals
			(id, event_id, device_id, vendor_id, tag_uid, amount_cents, reason, refused_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO NOTHING
	`, id, eventID, deviceID, vendorID, tagUID, amountCents, reason, at)
	return err
}

// LoadOfflinePolicy reads an event's caps from event_cashless_config.
//
// A MISSING row returns the ZERO OfflinePolicy, which denies everything — the
// safe default established in 000087. Never substitute built-in caps here: an
// organiser who never configured offline spend must get an event where nothing
// is approved offline, and a helpful fallback would silently undo that.
func LoadOfflinePolicy(ctx context.Context, pool *pgxpool.Pool, eventID string) (OfflinePolicy, error) {
	var (
		p          OfflinePolicy
		policy     string
		decayJSON  []byte
	)
	err := pool.QueryRow(ctx, `
		SELECT offline_policy::text, max_offline_taps, max_offline_spend_cents,
		       max_distinct_vendors, decay_steps, unknown_tag_allowance_cents
		FROM event_cashless_config WHERE event_id = $1
	`, eventID).Scan(&policy, &p.MaxOfflineTaps, &p.MaxOfflineSpendCents,
		&p.MaxDistinctVendors, &decayJSON, &p.UnknownTagAllowanceCents)
	if errors.Is(err, pgx.ErrNoRows) {
		return OfflinePolicy{}, nil // denies everything
	}
	if err != nil {
		return OfflinePolicy{}, err
	}

	p.Enabled = policy == "enabled"
	if len(decayJSON) > 0 {
		if err := json.Unmarshal(decayJSON, &p.DecaySteps); err != nil {
			// Malformed decay steps mean the tightening ceiling is not
			// enforceable. Returning the zero policy denies rather than falling
			// back to a flat cap, because a flat cap is exactly the v3.0
			// behaviour the decay was added to replace.
			return OfflinePolicy{}, fmt.Errorf("event %s has malformed decay_steps: %w", eventID, err)
		}
	}
	return p, nil
}