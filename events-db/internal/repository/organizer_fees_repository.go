package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Per-organiser fee terms (migration 000093) ───────────────────────────────
//
// An organiser either has a negotiated arrangement (custom_commission_pct and/or
// custom_service_fee set) or follows the platform defaults defined beside
// CalculateFees in booking_repository.go. Each column is independent: an
// organiser can have a custom commission and the default service fee.
//
// Changing an organiser's terms only affects bookings created afterwards. Every
// booking stores its own platform_fee and organizer_fee, and everything
// downstream — the Paystack split, payouts, refunds, analytics — reads those
// stored amounts, never the rate.

var (
	ErrFeeOrganizerNotFound = errors.New("organiser not found")
	ErrFeeCommissionRange   = fmt.Errorf("commission must be between %.2f%% and %.2f%%", MinOrganizerCommissionPct, MaxOrganizerCommissionPct)
	ErrFeeServiceFeeRange   = fmt.Errorf("service fee must be between R%.2f and R%.2f per ticket", MinServiceFeePerTicket, MaxServiceFeePerTicket)
	ErrFeeNoteRequired      = errors.New("a note explaining the arrangement is required")
	ErrFeeNoteTooLong       = errors.New("note must be 500 characters or fewer")
)

// FeeTerms are the two rates that price one booking line.
type FeeTerms struct {
	ServiceFeePerTicket float64 // ZAR per ticket, before the MaxServiceFeeRate cap
	CommissionRate      float64 // FRACTION of subtotal (0.05 = 5%)
}

// DefaultFeeTerms is what an organiser with no arrangement pays.
func DefaultFeeTerms() FeeTerms {
	return FeeTerms{
		ServiceFeePerTicket: ServiceFeePerTicket,
		CommissionRate:      OrganizerCommissionRate,
	}
}

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx, so fee terms can be
// read inside a booking transaction or outside one with the same function.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// GetFeeTermsForEvent resolves the fee terms of the organiser who owns an event.
//
// events.organizer_id holds a USER id (migration 000004), so the join is on
// organizers.user_id, not organizers.id.
//
// An event with no organiser row (admin-created, legacy data) gets the
// defaults. A genuine query error is returned rather than swallowed: silently
// falling back to the default would under- or over-charge an organiser with an
// arrangement, and inside a booking transaction the error aborts the tx anyway.
func GetFeeTermsForEvent(ctx context.Context, q rowQuerier, eventID string) (FeeTerms, error) {
	var customService, customCommissionPct *float64
	err := q.QueryRow(ctx, `
		SELECT o.custom_service_fee::float8, o.custom_commission_pct::float8
		FROM events e
		LEFT JOIN organizers o ON o.user_id = e.organizer_id
		WHERE e.id = $1
	`, eventID).Scan(&customService, &customCommissionPct)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DefaultFeeTerms(), nil
		}
		return FeeTerms{}, fmt.Errorf("failed to resolve fee terms: %w", err)
	}

	terms := DefaultFeeTerms()
	if customService != nil {
		terms.ServiceFeePerTicket = *customService
	}
	if customCommissionPct != nil {
		terms.CommissionRate = *customCommissionPct / 100
	}
	return terms, nil
}

// PublicServiceFeeTerms is what a buyer-facing client needs to show the service
// fee before checkout. The commission is deliberately absent: it is the
// organiser's commercial term, the buyer never pays it, and publishing it would
// let anyone read an organiser's deal off an event page.
type PublicServiceFeeTerms struct {
	PerTicket float64 `json:"per_ticket"` // ZAR, before the cap
	MaxRate   float64 `json:"max_rate"`   // cap as a fraction of face value
}

// PublicServiceFeeFor converts terms to the buyer-facing shape.
func PublicServiceFeeFor(t FeeTerms) PublicServiceFeeTerms {
	return PublicServiceFeeTerms{PerTicket: t.ServiceFeePerTicket, MaxRate: MaxServiceFeeRate}
}

// ── Admin listing ────────────────────────────────────────────────────────────

// OrganizerFeeRow is one organiser as the master-admin fee screen sees it.
// The custom_* pointers are nil when the organiser follows the default; the
// effective_* values are always populated, so the UI never has to know the
// defaults itself.
type OrganizerFeeRow struct {
	OrganizerID            string     `json:"organizer_id"`
	BusinessName           string     `json:"business_name"`
	BusinessEmail          string     `json:"business_email"`
	Status                 string     `json:"status"`
	CustomCommissionPct    *float64   `json:"custom_commission_pct"`
	CustomServiceFee       *float64   `json:"custom_service_fee"`
	EffectiveCommissionPct float64    `json:"effective_commission_pct"`
	EffectiveServiceFee    float64    `json:"effective_service_fee"`
	Note                   *string    `json:"note"`
	UpdatedAt              *time.Time `json:"updated_at"`
	UpdatedByEmail         *string    `json:"updated_by_email"`
}

// FeeDefaults is echoed to the admin screen so its limits and placeholders come
// from the server rather than a second copy of the constants.
type FeeDefaults struct {
	CommissionPct     float64 `json:"commission_pct"`
	ServiceFee        float64 `json:"service_fee"`
	MaxServiceFeeRate float64 `json:"max_service_fee_rate"`
	MinCommissionPct  float64 `json:"min_commission_pct"`
	MaxCommissionPct  float64 `json:"max_commission_pct"`
	MinServiceFee     float64 `json:"min_service_fee"`
	MaxServiceFee     float64 `json:"max_service_fee"`
}

func CurrentFeeDefaults() FeeDefaults {
	return FeeDefaults{
		CommissionPct:     round2(OrganizerCommissionRate * 100),
		ServiceFee:        ServiceFeePerTicket,
		MaxServiceFeeRate: MaxServiceFeeRate,
		MinCommissionPct:  MinOrganizerCommissionPct,
		MaxCommissionPct:  MaxOrganizerCommissionPct,
		MinServiceFee:     MinServiceFeePerTicket,
		MaxServiceFee:     MaxServiceFeePerTicket,
	}
}

func (r *OrganizerFeeRow) fillEffective() {
	r.EffectiveCommissionPct = round2(OrganizerCommissionRate * 100)
	if r.CustomCommissionPct != nil {
		r.EffectiveCommissionPct = *r.CustomCommissionPct
	}
	r.EffectiveServiceFee = ServiceFeePerTicket
	if r.CustomServiceFee != nil {
		r.EffectiveServiceFee = *r.CustomServiceFee
	}
}

const organizerFeeSelect = `
	SELECT o.id::text, o.business_name, o.business_email, o.status::text,
	       o.custom_commission_pct::float8, o.custom_service_fee::float8,
	       o.custom_fees_note, o.custom_fees_updated_at, u.email
	FROM organizers o
	LEFT JOIN users u ON u.id = o.custom_fees_updated_by
`

func scanOrganizerFeeRow(row pgx.Row) (OrganizerFeeRow, error) {
	var r OrganizerFeeRow
	err := row.Scan(
		&r.OrganizerID, &r.BusinessName, &r.BusinessEmail, &r.Status,
		&r.CustomCommissionPct, &r.CustomServiceFee,
		&r.Note, &r.UpdatedAt, &r.UpdatedByEmail,
	)
	if err != nil {
		return r, err
	}
	r.fillEffective()
	return r, nil
}

// ListOrganizerFees returns every organiser with its fee terms, organisers
// with an arrangement first, then alphabetically.
func ListOrganizerFees(ctx context.Context, pool *pgxpool.Pool) ([]OrganizerFeeRow, error) {
	rows, err := pool.Query(ctx, organizerFeeSelect+`
		ORDER BY (o.custom_commission_pct IS NOT NULL OR o.custom_service_fee IS NOT NULL) DESC,
		         LOWER(o.business_name) ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list organiser fees: %w", err)
	}
	defer rows.Close()

	out := []OrganizerFeeRow{}
	for rows.Next() {
		r, err := scanOrganizerFeeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan organiser fees: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── Admin update ─────────────────────────────────────────────────────────────

// FeeOverrideInput is one master-admin change. A nil field means "use the
// platform default" — it CLEARS the override rather than leaving it untouched,
// so the screen always sends the complete intended state.
type FeeOverrideInput struct {
	CommissionPct *float64
	ServiceFee    *float64
	Note          string
}

// ValidateFeeOverride enforces the same limits as the CHECK constraints in
// migration 000093, so a bad value is a clean 400 rather than a constraint
// violation surfacing as a 500.
func ValidateFeeOverride(in FeeOverrideInput) error {
	if in.CommissionPct != nil {
		v := *in.CommissionPct
		if math.IsNaN(v) || v < MinOrganizerCommissionPct || v > MaxOrganizerCommissionPct {
			return ErrFeeCommissionRange
		}
	}
	if in.ServiceFee != nil {
		v := *in.ServiceFee
		if math.IsNaN(v) || v < MinServiceFeePerTicket || v > MaxServiceFeePerTicket {
			return ErrFeeServiceFeeRange
		}
	}
	// A note is required whenever an arrangement exists — it is the only record
	// of WHY an organiser pays something different, and "who agreed this and
	// why" is the first question when the numbers are queried later. Resetting
	// to the defaults needs no justification.
	if (in.CommissionPct != nil || in.ServiceFee != nil) && len([]rune(strings.TrimSpace(in.Note))) < 3 {
		return ErrFeeNoteRequired
	}
	if len([]rune(strings.TrimSpace(in.Note))) > 500 {
		return ErrFeeNoteTooLong
	}
	return nil
}

// AuditActor is who made a change, captured by the handler on the request
// goroutine.
type AuditActor struct {
	UserID    string
	Email     string
	IP        string
	UserAgent string
}

// UpdateOrganizerFees applies a master-admin fee change and writes the audit
// row in the SAME transaction. Unlike the fire-and-forget AuditAllowed used for
// reads, a change to what an organiser is paid must never happen without its
// record: if the audit insert fails, the change rolls back.
func UpdateOrganizerFees(ctx context.Context, pool *pgxpool.Pool, organizerID string, in FeeOverrideInput, actor AuditActor) (before, after OrganizerFeeRow, err error) {
	if err = ValidateFeeOverride(in); err != nil {
		return before, after, err
	}

	// Round to the column scales so what is stored, audited and returned are
	// the same numbers.
	if in.CommissionPct != nil {
		v := round2(*in.CommissionPct)
		in.CommissionPct = &v
	}
	if in.ServiceFee != nil {
		v := round2(*in.ServiceFee)
		in.ServiceFee = &v
	}
	var note *string
	if n := strings.TrimSpace(in.Note); n != "" {
		note = &n
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return before, after, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	before, err = scanOrganizerFeeRow(tx.QueryRow(ctx, organizerFeeSelect+`
		WHERE o.id = $1
		FOR UPDATE OF o
	`, organizerID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return before, after, ErrFeeOrganizerNotFound
		}
		return before, after, fmt.Errorf("failed to load organiser: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE organizers
		SET custom_commission_pct  = $2,
		    custom_service_fee     = $3,
		    custom_fees_note       = $4,
		    custom_fees_updated_at = NOW(),
		    custom_fees_updated_by = $5
		WHERE id = $1
	`, organizerID, in.CommissionPct, in.ServiceFee, note, actor.UserID)
	if err != nil {
		return before, after, fmt.Errorf("failed to update organiser fees: %w", err)
	}

	after, err = scanOrganizerFeeRow(tx.QueryRow(ctx, organizerFeeSelect+`
		WHERE o.id = $1
	`, organizerID))
	if err != nil {
		return before, after, fmt.Errorf("failed to reload organiser: %w", err)
	}

	detail := map[string]any{
		"business_name": after.BusinessName,
		"before": map[string]any{
			"custom_commission_pct": before.CustomCommissionPct,
			"custom_service_fee":    before.CustomServiceFee,
			"effective_commission":  before.EffectiveCommissionPct,
			"effective_service_fee": before.EffectiveServiceFee,
		},
		"after": map[string]any{
			"custom_commission_pct": after.CustomCommissionPct,
			"custom_service_fee":    after.CustomServiceFee,
			"effective_commission":  after.EffectiveCommissionPct,
			"effective_service_fee": after.EffectiveServiceFee,
		},
		"note": note,
	}

	var ip *string
	if actor.IP != "" {
		ip = &actor.IP
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO staff_audit_log
			(actor_id, actor_email, actor_role, action, target_type, target_id,
			 allowed, detail, ip, user_agent)
		VALUES ($1, $2, (SELECT staff_role FROM users WHERE id = $1),
		        'organizer.fees_update', 'organizer', $3::uuid,
		        TRUE, $4, $5::inet, $6)
	`, actor.UserID, actor.Email, organizerID, detail, ip, actor.UserAgent)
	if err != nil {
		return before, after, fmt.Errorf("failed to write audit log: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return before, after, fmt.Errorf("failed to commit fee change: %w", err)
	}
	return before, after, nil
}