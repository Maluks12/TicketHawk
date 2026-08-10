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
	CapViewUserList     Capability = "user.list"          // names + roles only
	CapViewUserContact  Capability = "user.contact"       // email, phone
	CapViewBankTail     Capability = "bank.view_tail"     // last 4 digits only
	CapViewKYCStatus    Capability = "kyc.status"         // verified / pending
	CapViewKYCDocument  Capability = "kyc.document"       // the actual ID image
	CapViewAuditLog     Capability = "audit.view"

	// ── Acting ───────────────────────────────────────────────────────────────
	CapEditUserContact  Capability = "user.edit_contact"
	CapRequestOTP       Capability = "user.request_otp"
	CapResetCredentials Capability = "user.reset_credentials"
	CapApproveEvent     Capability = "event.approve"
	CapFeatureEvent     Capability = "event.feature"
	CapVerifyKYC        Capability = "kyc.verify"

	// ── Requesting (log a request; cannot execute) ───────────────────────────
	CapRequestDangerous Capability = "request.create"
	// ── Approving (second person on someone else's request) ──────────────────
	CapApproveRequests  Capability = "request.approve"

	// ── Staff management ─────────────────────────────────────────────────────
	CapViewStaff        Capability = "staff.view"
	CapManageStaff      Capability = "staff.manage"
)

// capabilities is the whole permission model, in one readable place.
//
// Anything not listed for a role is denied. That default matters: a new
// capability added below is automatically unavailable to every role until
// someone grants it deliberately, rather than leaking to whoever happens to sit
// above a numeric threshold.
var capabilities = map[StaffRole]map[Capability]bool{
	StaffReadOnly: {
		CapViewUserList: true,
	},
	StaffAccess: {
		CapViewUserList:    true,
		CapViewUserContact: true,
		CapViewBankTail:    true,
		CapViewKYCStatus:   true,
	},
	StaffEditor: {
		CapViewUserList:     true,
		CapViewUserContact:  true,
		CapViewBankTail:     true,
		CapViewKYCStatus:    true,
		CapEditUserContact:  true,
		CapRequestOTP:       true,
		CapResetCredentials: true,
		CapApproveEvent:     true,
		CapFeatureEvent:     true,
		// Editors may ASK for a takedown or a bank deletion. They may never
		// carry one out — that is the entire point of the split.
		CapRequestDangerous: true,
	},
	StaffAdmin: {
		CapViewUserList:     true,
		CapViewUserContact:  true,
		CapViewBankTail:     true,
		CapViewKYCStatus:    true,
		CapViewAuditLog:     true,
		CapEditUserContact:  true,
		CapRequestOTP:       true,
		CapResetCredentials: true,
		CapApproveEvent:     true,
		CapFeatureEvent:     true,
		CapVerifyKYC:        true,
		CapRequestDangerous: true,
		CapApproveRequests:  true,
		CapViewStaff:        true,
		CapManageStaff:      true,
		// CapViewKYCDocument is DELIBERATELY ABSENT.
		//
		// Not an oversight, and not something to "fix" later by adding a line:
		// no staff role can read an identity document through the console. KYC
		// verification is done against the DHA registry result, which is a
		// match/no-match answer plus the fields needed to judge it — it does
		// not require a human to look at a photograph of someone's ID.
		//
		// If a document genuinely must be reviewed by a person, that should be
		// a separate, individually-audited, time-boxed grant — not a standing
		// capability attached to a login that also approves payouts.
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