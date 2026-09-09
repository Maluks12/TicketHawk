package repository

import "strings"

// ── Refund authorisation ──────────────────────────────────────────────────────
//
// Pure. No database, no clock.
//
// A refund moves money OUT of the platform's float and back to an attendee, on
// the say-so of a vendor's till. The vendor has every incentive to be generous
// with money that is not theirs, and no incentive to check the arithmetic — so
// the constraints are not about catching malice so much as making the honest
// mistakes impossible: refunding twice because the first response was lost,
// refunding R500 against a R50 sale, refunding another vendor's transaction.
//
// The asymmetry with AuthorizeSpend is worth noting. A spend is bounded by the
// attendee's balance, which the attendee controls. A refund is bounded by the
// ORIGINAL SALE, which is a fact in the ledger — so these checks are absolute
// rather than a judgement about affordability.

// Refund refusal reasons. Stable strings; the till shows these to a cashier
// with somebody waiting, so each maps to a different thing they should do.
const (
	RefundOK             = ""
	RefundNoOriginal     = "original_transaction_not_found"
	RefundNotAPurchase   = "original_is_not_a_purchase"
	RefundWrongVendor    = "original_belongs_to_another_vendor"
	RefundExceedsSale    = "refund_exceeds_remaining_amount"
	RefundInvalidAmount  = "invalid_amount"
	RefundNoReason       = "reason_required"
	RefundWalletClosed   = "wallet_is_closed"
)

// RefundContext is the original sale plus what has already been given back,
// resolved from the database by the caller.
type RefundContext struct {
	Found bool

	// OriginalKind guards against refunding a top-up or, worse, a refund —
	// which would be a machine for printing money.
	OriginalKind string

	// OriginalCents is the positive magnitude of the sale.
	OriginalCents int64

	// AlreadyRefundedCents is the sum of every refund already issued against
	// this sale. Partial refunds are legitimate, so the check is cumulative.
	AlreadyRefundedCents int64

	// VendorID on the original sale. A refund must come from the vendor that
	// made the sale: refunding another stall's transaction would credit the
	// attendee out of the wrong vendor's settlement.
	VendorID string

	WalletStatus string
}

// AuthorizeRefund decides whether a refund may proceed.
//
// requestingVendorID is the vendor the till belongs to, resolved from the
// device token rather than supplied by the client.
func AuthorizeRefund(ctx RefundContext, requestingVendorID string, amountCents int64, reason string) string {
	if amountCents <= 0 {
		return RefundInvalidAmount
	}
	if strings.TrimSpace(reason) == "" {
		// Required, not encouraged. An attendee disputing this weeks later, or
		// an auditor asking why R200 moved, needs the answer attached to the
		// entry rather than reconstructed from who was on shift.
		return RefundNoReason
	}
	if !ctx.Found {
		return RefundNoOriginal
	}

	// Only a purchase can be reversed. Refunding a top-up would return money
	// the attendee already has as balance; refunding a refund would create
	// value out of nothing, repeatedly.
	if ctx.OriginalKind != TxnPurchase {
		return RefundNotAPurchase
	}

	if ctx.VendorID == "" || ctx.VendorID != requestingVendorID {
		return RefundWrongVendor
	}

	// A closed wallet cannot receive credit — the band is gone and the balance
	// has been settled. Frozen is deliberately NOT blocked: a frozen wallet is
	// under investigation, and refusing to return money the attendee is owed
	// while investigating them is the wrong way round.
	if ctx.WalletStatus == WalletClosed {
		return RefundWalletClosed
	}

	// Cumulative, so three partial refunds cannot exceed the sale between them.
	if ctx.AlreadyRefundedCents+amountCents > ctx.OriginalCents {
		return RefundExceedsSale
	}

	return RefundOK
}

// RemainingRefundableCents is how much of a sale can still be given back.
//
// Used to offer a partial refund rather than a flat refusal: if R30 of a R50
// sale has already gone back, a cashier trying to refund R50 should be told
// R20 remains, not simply "no".
func RemainingRefundableCents(ctx RefundContext) int64 {
	if !ctx.Found || ctx.OriginalKind != TxnPurchase {
		return 0
	}
	if r := ctx.OriginalCents - ctx.AlreadyRefundedCents; r > 0 {
		return r
	}
	return 0
}

// ── Manual adjustments ───────────────────────────────────────────────────────

// Adjustment refusal reasons.
const (
	AdjustmentOK            = ""
	AdjustmentInvalidAmount = "invalid_amount"
	AdjustmentNoReason      = "reason_required"
	AdjustmentTooLarge      = "amount_exceeds_limit"
	AdjustmentWouldGoNegative = "would_take_wallet_negative"
)

// MaxAdjustmentCents caps a single manual correction at R5,000.
//
// A fat-finger control, like MaxPurchaseCents. An adjustment is the one path
// that can move money in either direction on a manager's word alone, so it gets
// the tighter ceiling of the two.
const MaxAdjustmentCents int64 = 500_000

// AuthorizeAdjustment decides whether a manual correction may proceed.
//
// signedCents carries its own direction: positive credits the attendee,
// negative debits them. Unlike every other path, the caller chooses the sign —
// a correction has to be able to undo whatever went wrong.
func AuthorizeAdjustment(walletStatus string, balanceCents, signedCents int64, reason string) string {
	if signedCents == 0 {
		return AdjustmentInvalidAmount
	}
	if strings.TrimSpace(reason) == "" {
		return AdjustmentNoReason
	}

	magnitude := signedCents
	if magnitude < 0 {
		magnitude = -magnitude
	}
	if magnitude > MaxAdjustmentCents {
		return AdjustmentTooLarge
	}

	// A debit adjustment may not push a wallet negative. Only offline spend is
	// allowed to do that, because there the money is genuinely already gone —
	// a manager correcting a figure has no such excuse, and a negative wallet
	// would then be picked up by the shortfall detector as if it were fraud.
	if signedCents < 0 && balanceCents+signedCents < 0 {
		return AdjustmentWouldGoNegative
	}

	return AdjustmentOK
}