package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// REFUND & ADJUSTMENT TESTS
// =============================================================================
//
//	go test ./internal/repository/... -v -run "TestRefund|TestAdjustment"
//
// A refund moves money out of the float on a vendor till's say-so. These
// checks are absolute rather than judgements about affordability: the bound is
// the original sale, which is a fact in the ledger.

const testVendor = "vvvvvvvv-vvvv-vvvv-vvvv-vvvvvvvvvvvv"

func saleContext() RefundContext {
	return RefundContext{
		Found:         true,
		OriginalKind:  TxnPurchase,
		OriginalCents: 5000, // R50 sale
		VendorID:      testVendor,
		WalletStatus:  WalletActive,
	}
}

// ─── The happy path ──────────────────────────────────────────────────────────

func TestRefundAllowsFullAndPartial(t *testing.T) {
	assert.Equal(t, RefundOK, AuthorizeRefund(saleContext(), testVendor, 5000, "wrong item"))
	assert.Equal(t, RefundOK, AuthorizeRefund(saleContext(), testVendor, 1000, "one drink returned"))
}

func TestRefundIsCumulative(t *testing.T) {
	// Three partial refunds must not exceed the sale between them. Checking
	// each in isolation would let R30 + R30 come back on a R50 sale.
	c := saleContext()
	c.AlreadyRefundedCents = 3000

	assert.Equal(t, RefundOK, AuthorizeRefund(c, testVendor, 2000, "rest of the round"))
	assert.Equal(t, RefundExceedsSale, AuthorizeRefund(c, testVendor, 2001, "too much"))
	assert.Equal(t, int64(2000), RemainingRefundableCents(c))
}

func TestRefundFullyRefundedSaleHasNothingLeft(t *testing.T) {
	c := saleContext()
	c.AlreadyRefundedCents = 5000
	assert.Equal(t, RefundExceedsSale, AuthorizeRefund(c, testVendor, 1, "again"))
	assert.Equal(t, int64(0), RemainingRefundableCents(c))
}

// ─── What may be refunded ────────────────────────────────────────────────────

func TestRefundOnlyReversesPurchases(t *testing.T) {
	// Refunding a top-up returns money the attendee already holds as balance.
	// Refunding a REFUND creates value from nothing, repeatedly — a machine for
	// printing money if it were ever allowed.
	for _, kind := range []string{TxnTopup, TxnRefund, TxnPayout, TxnAdjustment} {
		c := saleContext()
		c.OriginalKind = kind
		assert.Equal(t, RefundNotAPurchase,
			AuthorizeRefund(c, testVendor, 100, "reason"), kind)
	}
}

func TestRefundRequiresTheOriginal(t *testing.T) {
	c := saleContext()
	c.Found = false
	assert.Equal(t, RefundNoOriginal, AuthorizeRefund(c, testVendor, 100, "reason"))
}

func TestRefundMustComeFromTheSellingVendor(t *testing.T) {
	// Refunding another stall's sale would credit the attendee out of the wrong
	// vendor's settlement — the bar pays for the food truck's mistake.
	assert.Equal(t, RefundWrongVendor,
		AuthorizeRefund(saleContext(), "someone-else", 100, "reason"))

	c := saleContext()
	c.VendorID = ""
	assert.Equal(t, RefundWrongVendor, AuthorizeRefund(c, testVendor, 100, "reason"))
}

// ─── Reason and amount ───────────────────────────────────────────────────────

func TestRefundRequiresAReason(t *testing.T) {
	// Required, not encouraged. An attendee disputing this weeks later needs
	// the answer attached to the entry.
	for _, r := range []string{"", "   ", "\t\n"} {
		assert.Equal(t, RefundNoReason, AuthorizeRefund(saleContext(), testVendor, 100, r))
	}
}

func TestRefundRejectsBadAmounts(t *testing.T) {
	assert.Equal(t, RefundInvalidAmount, AuthorizeRefund(saleContext(), testVendor, 0, "reason"))
	assert.Equal(t, RefundInvalidAmount, AuthorizeRefund(saleContext(), testVendor, -100, "reason"))
}

// ─── Wallet state ────────────────────────────────────────────────────────────

func TestRefundFrozenWalletStillReceivesMoney(t *testing.T) {
	// Deliberate asymmetry with spending. A frozen wallet is under
	// investigation — refusing to return money the attendee is OWED while
	// investigating them is the wrong way round.
	c := saleContext()
	c.WalletStatus = WalletFrozen
	assert.Equal(t, RefundOK, AuthorizeRefund(c, testVendor, 1000, "wrong item"))
}

func TestRefundClosedWalletIsRefused(t *testing.T) {
	// The band is gone and the balance has been settled; there is nowhere for
	// the money to land. Needs handling out of band.
	c := saleContext()
	c.WalletStatus = WalletClosed
	assert.Equal(t, RefundWalletClosed, AuthorizeRefund(c, testVendor, 1000, "wrong item"))
}

func TestRefundChecksOrderIsActionable(t *testing.T) {
	// A cashier with a queue should get the most useful reason first. An
	// amount problem is theirs to fix; a wrong-vendor problem is not.
	c := saleContext()
	c.Found = false
	assert.Equal(t, RefundInvalidAmount, AuthorizeRefund(c, "wrong-vendor", 0, ""),
		"a bad amount is reported before anything they cannot act on")
}

// ─── Adjustments ─────────────────────────────────────────────────────────────

func TestAdjustmentAllowsBothDirections(t *testing.T) {
	// The only path where the caller chooses the sign — a correction has to be
	// able to undo whatever went wrong.
	assert.Equal(t, AdjustmentOK, AuthorizeAdjustment(WalletActive, 10000, 5000, "goodwill"))
	assert.Equal(t, AdjustmentOK, AuthorizeAdjustment(WalletActive, 10000, -5000, "double charge"))
}

func TestAdjustmentCannotTakeAWalletNegative(t *testing.T) {
	// Only offline spend may push a wallet negative, because there the money is
	// genuinely already gone. A manager correcting a figure has no such excuse —
	// and the resulting negative would be picked up by the shortfall detector
	// as though it were offline fraud.
	assert.Equal(t, AdjustmentWouldGoNegative,
		AuthorizeAdjustment(WalletActive, 1000, -1001, "correction"))

	assert.Equal(t, AdjustmentOK,
		AuthorizeAdjustment(WalletActive, 1000, -1000, "correction"),
		"exactly to zero is fine")
}

func TestAdjustmentRequiresAReason(t *testing.T) {
	// An unexplained movement of somebody else's money. The database enforces
	// this too, in chk_adjustment_has_reason.
	for _, r := range []string{"", "  "} {
		assert.Equal(t, AdjustmentNoReason, AuthorizeAdjustment(WalletActive, 10000, 100, r))
	}
}

func TestAdjustmentFatFingerCap(t *testing.T) {
	// Tighter than MaxPurchaseCents: this is the one path that moves money in
	// either direction on a manager's word alone.
	assert.Equal(t, AdjustmentTooLarge,
		AuthorizeAdjustment(WalletActive, 100_000_00, MaxAdjustmentCents+1, "big"))
	assert.Equal(t, AdjustmentTooLarge,
		AuthorizeAdjustment(WalletActive, 100_000_00, -(MaxAdjustmentCents + 1), "big"))
	assert.Equal(t, AdjustmentOK,
		AuthorizeAdjustment(WalletActive, 100_000_00, MaxAdjustmentCents, "at the limit"))
}

func TestAdjustmentRejectsZero(t *testing.T) {
	assert.Equal(t, AdjustmentInvalidAmount, AuthorizeAdjustment(WalletActive, 10000, 0, "reason"))
}