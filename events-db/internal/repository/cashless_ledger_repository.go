package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

// ── Cashless ledger core ──────────────────────────────────────────────────────
//
// Pure. No database, no clock, no models import — the same shape as
// ClassifyTagScan, and for the same reason: this is where money decisions are
// made, and they should be provable without infrastructure.
//
// MONEY IS INTEGER CENTS IN GO, DECIMAL(10,2) IN POSTGRES.
//
// This deviates from the surrounding convention — ad_wallets, payments and
// merch all carry float64 through Go — and the deviation is deliberate. Those
// paths let Postgres do the arithmetic and use float64 only to move a value to
// a screen, which is harmless. A wallet balance is different: it is a SUM over
// thousands of rows, and it is COMPARED against a purchase on every tap. In
// float64, R0.10 + R0.20 is not R0.30, and a wallet holding exactly the price
// of a beer will sometimes refuse to buy it. Postgres DECIMAL is exact, so
// balances are summed there; everything Go compares or accumulates is int64
// cents, converted once at the boundary by ToCents/FromCents.
//
// If a later phase adds a spend path that does arithmetic on a float64 rand
// amount, that is a bug even when the tests pass — the error only appears at
// volume.

// Transaction kinds, mirroring the cashless_txn_kind enum. Local literals, as
// with bookingStatusConfirmed and the tag statuses in this package.
const (
	TxnTopup      = "topup"
	TxnPurchase   = "purchase"
	TxnRefund     = "refund"
	TxnPayout     = "payout"
	TxnAdjustment = "adjustment"
)

// Verification classes, mirroring tag_verification_class in 000085.
const verifyClassUnverified = "unverified"

// Wallet statuses, mirroring cashless_wallet_status.
const (
	WalletActive = "active"
	WalletFrozen = "frozen"
	WalletClosed = "closed"
)

// Refusal reasons. Stable strings — the POS shows these to a cashier with a
// queue behind them, so each maps to a distinct thing they can actually do.
const (
	SpendOK               = ""
	SpendWalletNotActive  = "wallet_not_active"
	SpendNoWallet         = "tag_has_no_wallet"
	SpendChipNotEligible  = "chip_not_eligible_for_wallet"
	SpendTagNotActive     = "tag_not_active"
	SpendInsufficient     = "insufficient_funds"
	SpendInvalidAmount    = "invalid_amount"
	SpendAmountTooLarge   = "amount_exceeds_limit"
)

// MaxPurchaseCents caps a single sale at R10,000.
//
// Not a fraud control — it is a fat-finger control. A cashier entering 5000
// instead of 50.00 on a busy bar should be stopped by the till, not discovered
// in reconciliation the next morning. DECIMAL(10,2) tops out far higher, so
// this is policy rather than a storage limit, and it lives here so both the
// online and (Phase 4) offline paths inherit it.
const MaxPurchaseCents int64 = 1_000_000

// ── Money conversion ─────────────────────────────────────────────────────────

// ToCents converts a rand amount from JSON into exact cents.
//
// math.Round matters. A JSON value of 37.50 arrives as a float64 that is not
// precisely 37.5, so int64(rands*100) truncates to 3749 often enough to be a
// real bug — a cent lost per transaction, always in the same direction, across
// an entire festival. Rounding to the nearest cent is both correct and
// unbiased.
func ToCents(rands float64) (int64, error) {
	if math.IsNaN(rands) || math.IsInf(rands, 0) {
		return 0, fmt.Errorf("amount is not a finite number")
	}
	c := math.Round(rands * 100)
	if math.Abs(c) > 9_999_999_99 { // DECIMAL(10,2) ceiling
		return 0, fmt.Errorf("amount out of range for DECIMAL(10,2)")
	}
	return int64(c), nil
}

// FromCents converts back for display and for the DECIMAL bind parameter.
func FromCents(cents int64) float64 { return float64(cents) / 100 }

// ── Sign rules ───────────────────────────────────────────────────────────────

// SignedCents applies the direction implied by kind to a magnitude, so callers
// pass a positive amount and cannot accidentally credit a purchase.
//
// The database enforces the same rule in chk_cashless_sign. Both exist on
// purpose: this one gives a useful error at the handler, that one guarantees no
// future code path can bypass it.
func SignedCents(kind string, magnitudeCents int64) (int64, error) {
	if magnitudeCents == 0 {
		return 0, fmt.Errorf("amount must not be zero")
	}
	switch kind {
	case TxnTopup, TxnRefund:
		if magnitudeCents < 0 {
			return 0, fmt.Errorf("%s amount must be positive", kind)
		}
		return magnitudeCents, nil
	case TxnPurchase, TxnPayout:
		if magnitudeCents < 0 {
			return 0, fmt.Errorf("%s amount must be given as a positive magnitude", kind)
		}
		return -magnitudeCents, nil
	case TxnAdjustment:
		// The only kind that may go either way, because a correction has to be
		// able to undo whatever went wrong. Callers pass the sign they mean.
		return magnitudeCents, nil
	default:
		return 0, fmt.Errorf("unknown transaction kind %q", kind)
	}
}

// ── Spend authorisation ──────────────────────────────────────────────────────

// SpendContext is everything needed to decide one purchase, resolved from the
// database by the caller.
type SpendContext struct {
	TagStatus         string // rfid_tags.status
	VerificationClass string // rfid_tags.verification_class
	WalletID          *string
	WalletStatus      string
	BalanceCents      int64
}

// AuthorizeSpend decides whether a purchase may proceed. Returns SpendOK ("")
// or a refusal reason.
//
// Order is deliberate: the cheapest and most categorical checks first, so a
// revoked band or an ineligible chip is refused before any thought is given to
// money. It also means the reason a cashier sees is the most actionable one —
// "this band was reported lost" rather than "insufficient funds" on a band that
// should not be spending at all.
func AuthorizeSpend(ctx SpendContext, amountCents int64) string {
	if amountCents <= 0 {
		return SpendInvalidAmount
	}
	if amountCents > MaxPurchaseCents {
		return SpendAmountTooLarge
	}

	// Chip eligibility before anything else. chk_rfid_wallet_requires_verifiable_chip
	// already makes a uid_only wallet binding unrepresentable, so reaching this
	// means the constraint was dropped or the data predates it. Refuse rather
	// than trust.
	if ctx.VerificationClass == verifyClassUnverified {
		return SpendChipNotEligible
	}
	if ctx.TagStatus != tagStatusActive {
		return SpendTagNotActive
	}
	if ctx.WalletID == nil || strings.TrimSpace(*ctx.WalletID) == "" {
		return SpendNoWallet
	}
	if ctx.WalletStatus != WalletActive {
		return SpendWalletNotActive
	}

	// Online, this is absolute: the balance is the ledger sum read in the same
	// transaction that writes the debit, so there is no window to overspend
	// through. Phase 4's offline caps sit ON TOP of this, they do not replace
	// it — an offline device still refuses what it can see it cannot afford.
	if ctx.BalanceCents < amountCents {
		return SpendInsufficient
	}

	return SpendOK
}

// ── Hash chain ───────────────────────────────────────────────────────────────

// ChainHash computes a row's hash over its own fields plus the previous row's
// hash, making the wallet's history tamper-evident: editing or deleting any
// past entry invalidates every hash after it.
//
// This detects tampering, it does not prevent it — anyone who can rewrite the
// table can recompute the chain. Its value is that a partial edit (the ordinary
// case: someone patches one row to hide a charge) becomes visible, and that the
// chain can be re-verified independently later. Keyed hashing would raise the
// bar, but the key would have to live where the ledger writer can reach it, so
// it buys less than it looks.
//
// The field list is frozen. Changing what goes in, or the order, invalidates
// every existing chain — so a new field means a version prefix, not an edit.
func ChainHash(prevHash, walletID, kind string, seq, signedCents int64, vendorID, tagUID string) string {
	h := sha256.New()
	// Length-prefixed so no combination of field values can be rearranged into
	// the same byte stream — without this, a vendor id ending in a digit and a
	// tag uid starting with one could collide with a different pairing.
	for _, f := range []string{
		"chv1",
		prevHash,
		walletID,
		kind,
		fmt.Sprintf("%d", seq),
		fmt.Sprintf("%d", signedCents),
		vendorID,
		tagUID,
	} {
		fmt.Fprintf(h, "%d:%s|", len(f), f)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyChain walks a wallet's entries in sequence order and reports the first
// index whose hash does not match, or -1 when the chain is intact.
//
// Used by the reconciliation report rather than on the hot path — verifying is
// O(history) and a purchase must not pay that cost.
type ChainEntry struct {
	WalletID    string
	Kind        string
	Seq         int64
	SignedCents int64
	VendorID    string
	TagUID      string
	PrevHash    string
	Hash        string
}

func VerifyChain(entries []ChainEntry) int {
	prev := ""
	for i, e := range entries {
		if e.PrevHash != prev {
			return i
		}
		if ChainHash(prev, e.WalletID, e.Kind, e.Seq, e.SignedCents, e.VendorID, e.TagUID) != e.Hash {
			return i
		}
		prev = e.Hash
	}
	return -1
}