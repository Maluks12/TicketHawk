package models

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// ── Event visibility ─────────────────────────────────────────────────────────

type EventVisibility string

const (
	VisibilityPublic      EventVisibility = "public"
	VisibilityPrivateLink EventVisibility = "private_link"
)

// RightsDeclarationVersion identifies the wording of the event-rights
// declaration the organizer accepted. Bump when the legal text changes so
// old acceptances remain auditable against the text they actually saw.
const RightsDeclarationVersion = 1

// ── KYC statuses ─────────────────────────────────────────────────────────────

type KYCStatus string

const (
	KYCNone          KYCStatus = "none"
	KYCPendingReview KYCStatus = "pending_review"
	KYCVerified      KYCStatus = "verified"
	KYCRejected      KYCStatus = "rejected"
)

const (
	KYCMethodManual = "manual"
	KYCMethodDidit  = "didit"
)

// ── KYC tiers ────────────────────────────────────────────────────────────────
// Tier 1 (basic): legal name + SA ID number, locally validated. Grants the
// organiser role; unlocks private free events.
// Tier 2 (full): + ID document + registry verification. Unlocks paid & public.
const (
	KYCLevelNone  = 0
	KYCLevelBasic = 1
	KYCLevelFull  = 2
)

// MinLevelFor is the KYC tier an organiser must hold to PUBLISH an event of
// the given class. Private free → basic. Anything paid, or anything public
// (feed-visible), → full.
func MinLevelFor(visibility EventVisibility, isPaid bool) int {
	if isPaid || visibility == VisibilityPublic {
		return KYCLevelFull
	}
	return KYCLevelBasic
}

// KYCDocMaxBytes caps uploaded identity documents (ID photo, proof of
// address). They're stored as BYTEA, so the cap protects the DB.
const KYCDocMaxBytes = 5 << 20 // 5 MiB

// ── Requirements matrix (design doc section 4) ───────────────────────────────

// EventRequirements is what an organizer must have in place before an event
// of a given class can be published. Free public events follow the free
// private column for KYC (the matrix only distinguishes paid classes) but
// keep the existing listing-fee and admin-review rules.
type EventRequirements struct {
	KYCVerifiedRequired    bool `json:"kyc_verified_required"`
	BankDetailsRequired    bool `json:"bank_details_required"`
	RightsDeclaration      bool `json:"rights_declaration_required"`
	GoesThroughAdminReview bool `json:"goes_through_admin_review"`
	ListingFeeApplies      bool `json:"listing_fee_applies"`
}

// RequirementsFor evaluates the matrix for one event.
//
//	Free  private_link → declaration only; instant publish; no listing fee.
//	Free  public       → declaration; existing listing fee + admin review.
//	Paid  private_link → KYC + bank + declaration; instant publish.
//	Paid  public       → KYC + bank + declaration; admin review (existing flow).
//
// NOTE: private_link events are exempt from the free-event listing fee. The
// fee monetizes feed placement, which private events never get. If organizers
// abuse "private" to dodge the fee while sharing links widely, flip
// ListingFeeApplies to true for the free+private case — one line.
func RequirementsFor(visibility EventVisibility, isPaid bool) EventRequirements {
	private := visibility == VisibilityPrivateLink
	return EventRequirements{
		// "Verified" here means FULL KYC (tier 2). Public events now require
		// it as well as paid ones — feed visibility demands a verified human.
		KYCVerifiedRequired:    isPaid || !private,
		BankDetailsRequired:    isPaid,
		RightsDeclaration:      true,
		GoesThroughAdminReview: !private,
		ListingFeeApplies:      !private,
	}
}

// ── SA ID number validation ──────────────────────────────────────────────────

// ValidateSAIDNumber checks a 13-digit South African ID number:
// digits-only, valid YYMMDD birth date, citizenship digit ∈ {0,1}, and the
// Luhn check digit. Returns the derived date of birth on success.
//
// Century inference: SA IDs carry a two-digit year. A YY that would put the
// birth date in the future belongs to the 1900s. Organizers must be 18+.
func ValidateSAIDNumber(id string, now time.Time) (dob time.Time, err error) {
	id = strings.TrimSpace(id)
	if len(id) != 13 {
		return dob, fmt.Errorf("ID number must be exactly 13 digits")
	}
	for _, r := range id {
		if !unicode.IsDigit(r) {
			return dob, fmt.Errorf("ID number must contain digits only")
		}
	}

	// Citizenship digit (index 10): 0 = SA citizen, 1 = permanent resident.
	if id[10] != '0' && id[10] != '1' {
		return dob, fmt.Errorf("ID number has an invalid citizenship digit")
	}

	// Luhn over all 13 digits (check digit is the 13th).
	sum := 0
	double := false
	for i := len(id) - 1; i >= 0; i-- {
		d := int(id[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	if sum%10 != 0 {
		return dob, fmt.Errorf("ID number failed its checksum — please double-check the digits")
	}

	// Birth date.
	yy := int(id[0]-'0')*10 + int(id[1]-'0')
	mm := int(id[2]-'0')*10 + int(id[3]-'0')
	dd := int(id[4]-'0')*10 + int(id[5]-'0')

	year := 2000 + yy
	candidate := time.Date(year, time.Month(mm), mm2dayGuard(dd), 0, 0, 0, 0, time.UTC)
	if candidate.After(now) {
		year = 1900 + yy
	}
	dob = time.Date(year, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	// Reject impossible dates (time.Date normalizes, e.g. Feb 30 → Mar 2).
	if int(dob.Month()) != mm || dob.Day() != dd || mm < 1 || mm > 12 {
		return dob, fmt.Errorf("ID number encodes an invalid birth date")
	}

	if age := yearsBetween(dob, now); age < 18 {
		return dob, fmt.Errorf("organizers must be at least 18 years old")
	} else if age > 120 {
		return dob, fmt.Errorf("ID number encodes an implausible birth date")
	}
	return dob, nil
}

func mm2dayGuard(dd int) int {
	if dd < 1 {
		return 1
	}
	return dd
}

func yearsBetween(from, to time.Time) int {
	years := to.Year() - from.Year()
	if to.YearDay() < from.YearDay() {
		years--
	}
	return years
}

// ── Name matching (bank account vs legal name) ───────────────────────────────

// NameMatchResult grades how well the payout bank-account holder name matches
// the legal name on the ID. Computed at read time (never stored) so it can't
// go stale when either side changes.
type NameMatchResult string

const (
	NameMatchFull    NameMatchResult = "match"
	NameMatchPartial NameMatchResult = "partial"
	NameMatchNone    NameMatchResult = "none"
	NameMatchUnknown NameMatchResult = "unknown" // one side missing
)

// MatchNames compares a bank account holder name against legal first+last.
// Normalized token comparison: full = same token set; partial = surname
// present plus first-initial agreement or subset relationship.
func MatchNames(bankAccountName, legalFirst, legalLast string) NameMatchResult {
	bank := nameTokens(bankAccountName)
	legal := nameTokens(legalFirst + " " + legalLast)
	if len(bank) == 0 || len(legal) == 0 {
		return NameMatchUnknown
	}

	if sameTokenSet(bank, legal) {
		return NameMatchFull
	}

	// Surname present in the bank name is enough for "partial": covers
	// initials ("L MALUKS"), extra middle names, and sole-prop business
	// accounts ("MALUKS TRADING CC"). This is an advisory grade for the
	// reviewer, not a hard gate — the admin makes the call.
	surname := strings.ToUpper(strings.TrimSpace(legalLast))
	for _, t := range bank {
		if t == surname {
			return NameMatchPartial
		}
	}
	return NameMatchNone
}

func nameTokens(s string) []string {
	var out []string
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func sameTokenSet(a, b []string) bool {
	return isSubset(a, b) && isSubset(b, a)
}

func isSubset(sub, super []string) bool {
	set := make(map[string]bool, len(super))
	for _, t := range super {
		set[t] = true
	}
	for _, t := range sub {
		if !set[t] && !initialOf(t, super) {
			return false
		}
	}
	return true
}

// initialOf treats a single-letter token as matching any token it initials
// ("J SMITH" vs "JOHN SMITH").
func initialOf(tok string, super []string) bool {
	if len(tok) != 1 {
		return false
	}
	for _, t := range super {
		if strings.HasPrefix(t, tok) {
			return true
		}
	}
	return false
}


// ── DTOs / views ─────────────────────────────────────────────────────────────

// SubmitKYCDTO — non-file fields of the multipart KYC submission.
// (id_document / proof_of_address arrive as multipart files.)
//
// SAIDNumber is deliberately `omitempty`, NOT `required`: when the organiser
// already has an ID number on file (Level 1 basic KYC stores one), the client
// omits the field entirely and KYCService.Submit decrypts the stored value and
// reuses it. Marking this `required` made that fallback unreachable — the bind
// failed with a 400 before the service ever ran, so "use the ID on my profile"
// always failed while re-typing the identical number worked. `len=13` still
// applies whenever a value IS supplied, so a malformed number can't slip past;
// full checksum/DOB/age validation happens in ValidateSAIDNumber either way.
type SubmitKYCDTO struct {
	LegalFirstName string `form:"legal_first_name" binding:"required,min=2,max=100"`
	LegalLastName  string `form:"legal_last_name" binding:"required,min=2,max=100"`
	SAIDNumber     string `form:"sa_id_number" binding:"omitempty,len=13"`
}

// KYCProfile — organizer-facing view of their KYC state.
type KYCProfile struct {
	Status          KYCStatus  `json:"status"`
	Method          *string    `json:"method,omitempty"`
	LegalFirstName  *string    `json:"legal_first_name,omitempty"`
	LegalLastName   *string    `json:"legal_last_name,omitempty"`
	IDLast4         *string    `json:"id_last_4,omitempty"`
	SubmittedAt     *time.Time `json:"submitted_at,omitempty"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	HasIDDocument   bool       `json:"has_id_document"`
	HasProofOfAddr  bool       `json:"has_proof_of_address"`
}

// AdminKYCDetail — admin review view. Includes the decrypted ID number so the
// reviewer can compare it against the uploaded document; never sent anywhere
// but the authenticated admin endpoint.
type AdminKYCDetail struct {
	OrganizerID     string          `json:"organizer_id"`
	BusinessName    string          `json:"business_name"`
	BusinessEmail   string          `json:"business_email"`
	Status          KYCStatus       `json:"status"`
	Method          *string         `json:"method,omitempty"`
	LegalFirstName  *string         `json:"legal_first_name,omitempty"`
	LegalLastName   *string         `json:"legal_last_name,omitempty"`
	SAIDNumber      *string         `json:"sa_id_number,omitempty"` // decrypted, admin eyes only
	DateOfBirth     *string         `json:"date_of_birth,omitempty"`
	SubmittedAt     *time.Time      `json:"submitted_at,omitempty"`
	VerifiedAt      *time.Time      `json:"verified_at,omitempty"`
	RejectionReason *string         `json:"rejection_reason,omitempty"`
	HasIDDocument   bool            `json:"has_id_document"`
	HasProofOfAddr  bool            `json:"has_proof_of_address"`
	BankAccountName *string         `json:"bank_account_name,omitempty"`
	NameMatch       NameMatchResult `json:"name_match"`
	RegistryCheck   interface{}     `json:"registry_check,omitempty"`
	RegistryAt      *time.Time      `json:"registry_checked_at,omitempty"`

	// ── Document visibility, computed per caller ─────────────────────────────
	//
	// Not a property of the submission — a property of the submission AND the
	// person asking. Two reviewers open the same organiser and get different
	// values here, which is the point: the console draws what the server would
	// actually allow instead of guessing from a role.

	// FirstVerifiedAt is when KYC completed for the FIRST time. Once set, the
	// ID photograph is master-admin-only. Surfaced so the screen can say
	// "locked since 3 June" rather than just "locked".
	FirstVerifiedAt *time.Time `json:"kyc_first_verified_at,omitempty"`
	// DocumentLocked mirrors FirstVerifiedAt != nil.
	DocumentLocked bool `json:"document_locked"`
	// CanViewDocument is the answer for this caller, this organiser, right now.
	CanViewDocument bool `json:"can_view_document"`
	// DocumentViaGrant means the access is borrowed and expiring.
	DocumentViaGrant bool `json:"document_access_via_grant"`
	// DocumentGrantExpiresAt, when DocumentViaGrant.
	DocumentGrantExpiresAt *time.Time `json:"document_grant_expires_at,omitempty"`
	// DocumentLockReason is shown verbatim when CanViewDocument is false. A
	// reviewer who sees an empty panel assumes the upload failed and rejects a
	// valid submission — the reason is what stops that.
	DocumentLockReason string `json:"document_lock_reason,omitempty"`
}

// AdminKYCDecisionDTO — approve or reject a pending submission.
type AdminKYCDecisionDTO struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason" binding:"max=500"`
}