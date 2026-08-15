package models

import "fmt"

// ─────────────────────────────────────────────────────────────────────────────
// Staff roles and capabilities
//
// StaffRole is ORTHOGONAL to UserRole. UserRole says what an account is on the
// platform (attendee, organizer, admin); StaffRole says what a human may do
// inside the internal console. Keeping them separate means AdminOnlyMiddleware
// keeps meaning exactly what it means today, and a support agent who also buys
// tickets needs one account rather than two.
//
// CAPABILITIES ARE ENUMERATED, NOT INFERRED FROM RANK
//
// The obvious design is a numeric level with `if user.Level >= 3`. It is also
// the design that produces the wrong answer here, because this permission set
// is NOT a straight ladder:
//
//	StaffAdmin outranks everyone AND STILL CANNOT read ID documents or full bank
//	numbers.
//
// That rule exists because the point of the console is to administer the
// platform, not to browse identity documents — and the highest-privilege
// account is the most valuable one to steal, so it should hold the least
// sensitive data. Under `>=` that exception is impossible to express: any
// threshold low enough to let editors work lets admins read too. Enumerating
// each capability makes the exception a line of code instead of a special case
// someone has to remember.
// ─────────────────────────────────────────────────────────────────────────────

type StaffRole string

const (
	// StaffReadOnly sees that accounts exist and nothing about them.
	StaffReadOnly StaffRole = "read_only"
	// StaffAccess adds contact details and KYC status. Still cannot change anything.
	StaffAccess StaffRole = "access"
	// StaffEditor can act on accounts, and can LOG requests for dangerous
	// actions — but cannot approve them.
	StaffEditor StaffRole = "editor"
	// StaffAdmin approves requests and manages staff. Cannot read ID documents
	// or full bank numbers.
	StaffAdmin StaffRole = "admin"
)

func ParseStaffRole(s string) (StaffRole, error) {
	switch StaffRole(s) {
	case StaffReadOnly, StaffAccess, StaffEditor, StaffAdmin:
		return StaffRole(s), nil
	default:
		return "", fmt.Errorf("unknown staff role %q", s)
	}
}

// Capability is a single thing a staff member may do. Named for the ACTION, not
// the screen — screens get rebuilt, and a permission tied to "the users page"
// becomes meaningless the moment that page is split in two.
type Capability string

const (
	// ── Reading ──────────────────────────────────────────────────────────────
	CapViewEvents       Capability = "event.view"         // event list + full detail
	CapViewUserList     Capability = "user.list"          // names + roles only
	CapViewUserContact  Capability = "user.contact"       // email, phone
	CapViewBankTail     Capability = "bank.view_tail"     // last 4 digits only
	CapViewKYCStatus    Capability = "kyc.status"         // verified / pending
	CapViewAuditLog     Capability = "audit.view"

	// CapViewKYCDocument is held by NO role, including master admin.
	//
	// Master admin reaches documents through is_master_admin, and admin/editor
	// through a time-boxed grant a master admin approves. Neither is a standing
	// capability, so it cannot be acquired by being promoted.
	CapViewKYCDocument Capability = "kyc.document"

	// ── Acting ───────────────────────────────────────────────────────────────
	CapEditEvents       Capability = "event.edit"
	CapApproveEvent     Capability = "event.approve"
	CapFeatureEvent     Capability = "event.feature"
	CapVerifyKYC        Capability = "kyc.verify"         // initial approval, on status
	CapEditUserContact  Capability = "user.edit_contact"
	CapRequestOTP       Capability = "user.request_otp"
	CapResetCredentials Capability = "user.reset_credentials"

	// ── Requesting vs doing ──────────────────────────────────────────────────
	// Editors LOG dangerous actions; admins perform them outright.
	CapRequestDangerous Capability = "request.create"
	CapActDangerous     Capability = "request.act"
	CapApproveRequests  Capability = "request.approve"

	// ── Staff management ─────────────────────────────────────────────────────
	CapViewStaff   Capability = "staff.view"
	CapManageStaff Capability = "staff.manage"
)

// capabilities is the whole permission model, in one readable place.
//
// Anything not listed for a role is denied, so a capability added below is
// unavailable until granted deliberately rather than leaking to whoever happens
// to sit above a numeric threshold.
var capabilities = map[StaffRole]map[Capability]bool{
	// Sees that events and accounts exist. Nothing personal, no writes.
	StaffReadOnly: {
		CapViewEvents:   true,
		CapViewUserList: true,
	},

	// Adds contact details, KYC status and the bank tail, plus initial KYC
	// approval. Still cannot edit events or touch accounts.
	StaffAccess: {
		CapViewEvents:      true,
		CapViewUserList:    true,
		CapViewUserContact: true,
		CapViewBankTail:    true,
		CapViewKYCStatus:   true,
		CapVerifyKYC:       true,
	},

	// Acts on events and accounts, and LOGS dangerous actions for an admin to
	// approve. Never carries one out — that split is the whole point.
	StaffEditor: {
		CapViewEvents:       true,
		CapViewUserList:     true,
		CapViewUserContact:  true,
		CapViewBankTail:     true,
		CapViewKYCStatus:    true,
		CapVerifyKYC:        true,
		CapEditEvents:       true,
		CapApproveEvent:     true,
		CapFeatureEvent:     true,
		CapEditUserContact:  true,
		CapRequestOTP:       true,
		CapResetCredentials: true,
		CapRequestDangerous: true,
	},

	// Everything an editor can do, without needing approval, plus approving
	// other people's requests and managing staff.
	StaffAdmin: {
		CapViewEvents:       true,
		CapViewUserList:     true,
		CapViewUserContact:  true,
		CapViewBankTail:     true,
		CapViewKYCStatus:    true,
		CapViewAuditLog:     true,
		CapVerifyKYC:        true,
		CapEditEvents:       true,
		CapApproveEvent:     true,
		CapFeatureEvent:     true,
		CapEditUserContact:  true,
		CapRequestOTP:       true,
		CapResetCredentials: true,
		CapRequestDangerous: true,
		CapActDangerous:     true,
		CapApproveRequests:  true,
		CapViewStaff:        true,
		CapManageStaff:      true,
	},
}

// Can reports whether a role holds a capability.
//
// Takes a POINTER so "not staff" and "staff with no permissions" are different
// values that both correctly deny, rather than the empty string silently
// matching a map entry someone adds later.
func Can(role *StaffRole, c Capability) bool {
	if role == nil {
		return false
	}
	caps, ok := capabilities[*role]
	if !ok {
		return false
	}
	return caps[c]
}

// Capabilities lists everything a role holds. Used by the console to render the
// UI it can actually use — the frontend asks the server what it may do rather
// than keeping its own copy of this table, so the two can never disagree.
func Capabilities(role *StaffRole) []Capability {
	if role == nil {
		return []Capability{}
	}
	out := make([]Capability, 0, len(capabilities[*role]))
	for c, ok := range capabilities[*role] {
		if ok {
			out = append(out, c)
		}
	}
	return out
}

// ── Request kinds and the gates each one must clear ──────────────────────────

type StaffRequestKind string

const (
	ReqBankDelete       StaffRequestKind = "bank_details_delete"
	ReqBankAdd          StaffRequestKind = "bank_details_add"
	ReqEventTakedown    StaffRequestKind = "event_takedown"
	ReqRoleChange       StaffRequestKind = "role_change"
	ReqCredentialReset  StaffRequestKind = "credential_reset"
	ReqAccountDelete    StaffRequestKind = "account_delete"
)

// RequestPolicy is how much friction a given action carries.
type RequestPolicy struct {
	// SubjectMustConfirm requires the affected person to approve by OTP.
	SubjectMustConfirm bool
	// CoolingOff delays execution after approval, giving the subject a window
	// to notice and object.
	CoolingOffMinutes int
	// PayoutHoldHours freezes settlement after execution.
	PayoutHoldHours int
	// Description is shown to the approver so the stakes are on screen at the
	// moment of the decision, not in a runbook nobody opens.
	Description string
}

// RequestPolicies grades friction by BLAST RADIUS, not by how annoying the
// action is to perform.
var RequestPolicies = map[StaffRequestKind]RequestPolicy{
	// The most dangerous action in the product: it is how ticket money gets
	// redirected. Everything is switched on.
	ReqBankAdd: {
		SubjectMustConfirm: true,
		CoolingOffMinutes:  60,
		PayoutHoldHours:    48,
		Description: "Adds new payout bank details. Money will be sent to this " +
			"account. Requires the organiser's confirmation and holds payouts for 48h.",
	},
	// Deleting details cannot redirect money — worst case it stops a payout,
	// which is recoverable. Confirmed by the subject, but no hold.
	ReqBankDelete: {
		SubjectMustConfirm: true,
		CoolingOffMinutes:  30,
		Description: "Removes the organiser's payout bank details. Payouts will " +
			"fail until new details are added.",
	},
	// Affects buyers who already hold tickets, so it is not reversible in any
	// meaningful sense — but the organiser is not the right approver, since a
	// takedown is often *because of* the organiser.
	ReqEventTakedown: {
		SubjectMustConfirm: false,
		CoolingOffMinutes:  0,
		Description: "Unpublishes a live event. Ticket holders keep their tickets " +
			"and the event disappears from the feed.",
	},
	ReqRoleChange: {
		SubjectMustConfirm: false,
		CoolingOffMinutes:  0,
		Description: "Changes what this account can do on the platform.",
	},
	// The account is presumed compromised, so the subject cannot be asked —
	// whoever is holding the account may be the attacker.
	ReqCredentialReset: {
		SubjectMustConfirm: false,
		CoolingOffMinutes:  0,
		Description: "Forces a password reset. Use when an account is compromised.",
	},
	ReqAccountDelete: {
		SubjectMustConfirm: true,
		CoolingOffMinutes:  24 * 60,
		Description: "Permanently anonymises this account. Cannot be undone.",
	},
}