package models

// UpdateOrganizerAccountDTO — the organiser editing their OWN account.
//
// Distinct from UpdateOrganizerDTO, which is declared in organizer.go with
// `binding:"required"` on pointer fields. That combination cannot express a
// sparse patch: `required` on a pointer rejects the request when the key is
// absent, so every save would have to send every field, and a form holding a
// stale value would overwrite a change made in another tab.
//
// Every field here is optional. Omit a key to leave the column alone; send an
// empty website_url to clear it.
//
// Deliberately absent: slug (assigned once, see migration 000077), verified,
// kyc_level, status and commission_rate — none of which are the subject's to
// set — and description, which belongs to the public page and is written by
// UpdatePublicProfile.
type UpdateOrganizerAccountDTO struct {
	BusinessName  *string `json:"business_name"  binding:"omitempty,min=2,max=255"`
	BusinessEmail *string `json:"business_email" binding:"omitempty,email"`
	BusinessPhone *string `json:"business_phone" binding:"omitempty,min=6,max=32"`
	WebsiteURL    *string `json:"website_url"    binding:"omitempty,max=500"`
}