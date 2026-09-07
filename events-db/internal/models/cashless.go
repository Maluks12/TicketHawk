package models

import "time"

// ── Cashless wallets, vendors, and POS devices ────────────────────────────────
//
// Go side of the enums in migration 000086. A value added on one side and not
// the other fails at the pgx driver, not at compile time, so they move together.
//
// Money is float64 HERE because these structs are JSON on the way out and DECIMAL
// on the way in — Postgres does the arithmetic. Anything that ADDS or COMPARES
// money uses int64 cents via repository.ToCents; see the header of
// cashless_ledger.go for why that distinction is not cosmetic.

type VendorStatus string

const (
	VendorActive    VendorStatus = "active"
	VendorSuspended VendorStatus = "suspended"
	VendorClosed    VendorStatus = "closed"
)

type POSDeviceStatus string

const (
	POSActive    POSDeviceStatus = "active"
	POSSuspended POSDeviceStatus = "suspended"
	POSRetired   POSDeviceStatus = "retired"
)

type CashlessWalletStatus string

const (
	CashlessWalletActive CashlessWalletStatus = "active"
	CashlessWalletFrozen CashlessWalletStatus = "frozen"
	CashlessWalletClosed CashlessWalletStatus = "closed"
)

// WalletFunding records how a wallet has been funded so far. Phase 4's
// cash-only offline cap depends on it: a wallet with no card behind it has
// nothing to auto-charge if it goes negative, so its offline ceiling is zero.
// Maintained by trigger in the database, never by handlers.
type WalletFunding string

const (
	FundingNone  WalletFunding = "none"
	FundingCash  WalletFunding = "cash"
	FundingCard  WalletFunding = "card"
	FundingMixed WalletFunding = "mixed"
)

type CashlessTxnKind string

// Prefixed CashlessKind* rather than the shorter CashlessTopup etc., because
// CashlessTopup is already the name of the top-up STRUCT below and Go has one
// namespace for both. The prefix also reads better at call sites, where
// models.CashlessKindPurchase is unambiguous about what kind of thing it is.
const (
	CashlessKindTopup      CashlessTxnKind = "topup"
	CashlessKindPurchase   CashlessTxnKind = "purchase"
	CashlessKindRefund     CashlessTxnKind = "refund"
	CashlessKindPayout     CashlessTxnKind = "payout"
	CashlessKindAdjustment CashlessTxnKind = "adjustment"
)

type CashlessTopupMethod string

const (
	TopupCard CashlessTopupMethod = "card"
	TopupCash CashlessTopupMethod = "cash"
)

type CashlessTopupStatus string

const (
	TopupPending   CashlessTopupStatus = "pending"
	TopupCompleted CashlessTopupStatus = "completed"
	TopupFailed    CashlessTopupStatus = "failed"
	TopupCancelled CashlessTopupStatus = "cancelled"
)

// CashlessTopupRefPrefix namespaces gateway references for attendee wallet
// top-ups.
//
// It is NOT "topup_". That prefix is already claimed by the organiser ad wallet
// in PaymentService.ApplyVerifiedPayment, and reusing it would route a
// festival-goer's R200 into an organiser's advertising balance — a live money
// bug that no test of either feature alone would catch.
const CashlessTopupRefPrefix = "ctop_"

// ── Structs ──────────────────────────────────────────────────────────────────

type EventVendor struct {
	ID             string       `json:"id"`
	EventID        string       `json:"event_id"`
	Name           string       `json:"name"`
	Code           string       `json:"code"`
	ContactName    *string      `json:"contact_name,omitempty"`
	ContactEmail   *string      `json:"contact_email,omitempty"`
	ContactPhone   *string      `json:"contact_phone,omitempty"`
	CommissionRate *float64     `json:"commission_rate,omitempty"`
	Status         VendorStatus `json:"status"`
	CreatedBy      string       `json:"created_by"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type VendorProduct struct {
	ID        string    `json:"id"`
	VendorID  string    `json:"vendor_id"`
	Name      string    `json:"name"`
	Category  *string   `json:"category,omitempty"`
	Price     float64   `json:"price"`
	IsActive  bool      `json:"is_active"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type VendorPOSDevice struct {
	ID         string          `json:"id"`
	EventID    string          `json:"event_id"`
	VendorID   string          `json:"vendor_id"`
	Label      string          `json:"label"`
	Status     POSDeviceStatus `json:"status"`
	LastSeenAt *time.Time      `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`

	// Token is populated ONLY in the response to provisioning. Only its hash is
	// stored, so this is the single moment it can ever be shown; a lost token
	// means re-provisioning the till, not looking it up.
	Token string `json:"token,omitempty"`
}

type CashlessWallet struct {
	ID         string               `json:"id"`
	EventID    string               `json:"event_id"`
	UserID     *string              `json:"user_id,omitempty"`
	Status     CashlessWalletStatus `json:"status"`
	Currency   string               `json:"currency"`
	Funding    WalletFunding        `json:"funding"`
	CardOnFile bool                 `json:"card_on_file"`
	CreatedAt  time.Time            `json:"created_at"`

	// Balance is computed from the ledger, never stored. Present on reads only.
	Balance float64 `json:"balance"`
}

type CashlessTransaction struct {
	ID        string          `json:"id"`
	WalletID  string          `json:"wallet_id"`
	Kind      CashlessTxnKind `json:"kind"`
	Amount    float64         `json:"amount"` // signed: credits +, debits −
	VendorID  *string         `json:"vendor_id,omitempty"`
	VendorName *string        `json:"vendor_name,omitempty"`
	TagUID    *string         `json:"tag_uid,omitempty"`
	SeqNum    int64           `json:"seq_num"`
	Mode      string          `json:"mode"`
	Detail    map[string]any  `json:"detail,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type CashlessTopup struct {
	ID                string              `json:"id"`
	WalletID          string              `json:"wallet_id"`
	Amount            float64             `json:"amount"`
	Fee               float64             `json:"fee"`
	Method            CashlessTopupMethod `json:"method"`
	Status            CashlessTopupStatus `json:"status"`
	PaystackReference *string             `json:"paystack_reference,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	CompletedAt       *time.Time          `json:"completed_at,omitempty"`
}