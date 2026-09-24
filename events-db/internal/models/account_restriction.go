package models

// RestrictionKind is one kind of master-admin account restriction
// (migration 000094). These values ARE the account_restrictions.kind CHECK
// constraint's list — add to both together.
type RestrictionKind string

const (
	// Lockouts. The account cannot sign in or use anything, and is signed out
	// everywhere the moment one is applied.
	RestrictDisable RestrictionKind = "disable" // for a set number of days
	RestrictBan     RestrictionKind = "ban"     // until a master admin lifts it

	// Feature blocks. The account signs in normally but cannot do this one
	// thing, for a set number of days or indefinitely.
	RestrictBuyTickets   RestrictionKind = "buy_tickets"
	RestrictCreateEvents RestrictionKind = "create_events" // creating AND publishing drafts
	RestrictBankDetails  RestrictionKind = "bank_details"
	RestrictPayouts      RestrictionKind = "payouts"
)

// Limits shared by the API, the console (echoed in the list response so the
// screen never offers a value the API will refuse) and the CHECK constraints in
// 000094.
const (
	MaxRestrictionDays      = 365
	MinRestrictionReasonLen = 5
	MaxRestrictionReasonLen = 500
)

// RestrictionFeatures lists the blockable features in the order the console
// and the sign-in popup show them.
var RestrictionFeatures = []RestrictionKind{
	RestrictBuyTickets,
	RestrictCreateEvents,
	RestrictBankDetails,
	RestrictPayouts,
}

// IsLockout reports whether the kind locks the whole account out.
func (k RestrictionKind) IsLockout() bool {
	return k == RestrictDisable || k == RestrictBan
}

// IsFeature reports whether the kind blocks a single feature.
func (k RestrictionKind) IsFeature() bool {
	switch k {
	case RestrictBuyTickets, RestrictCreateEvents, RestrictBankDetails, RestrictPayouts:
		return true
	}
	return false
}

// Valid reports whether the database accepts this kind.
func (k RestrictionKind) Valid() bool {
	return k.IsLockout() || k.IsFeature()
}

// HoldsPayouts reports whether a restriction of this kind stops money leaving
// the platform for the account's organiser profile. A lockout holds payouts as
// well as the payouts block itself.
//
// The SQL in FetchPayableRows and splitForBooking spells the same list out as
// ('payouts', 'disable', 'ban'). Change all three together.
func (k RestrictionKind) HoldsPayouts() bool {
	return k == RestrictPayouts || k.IsLockout()
}

// Label is the plain-language name, used by the console and the sign-in popup.
func (k RestrictionKind) Label() string {
	switch k {
	case RestrictDisable:
		return "Disabled"
	case RestrictBan:
		return "Banned"
	case RestrictBuyTickets:
		return "Buying tickets"
	case RestrictCreateEvents:
		return "Creating and publishing events"
	case RestrictBankDetails:
		return "Changing bank details"
	case RestrictPayouts:
		return "Payouts"
	}
	return string(k)
}