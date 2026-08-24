package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateOrganizerAccount updates the organiser's own account details.
//
// ── What is NOT here, and why ───────────────────────────────────────────────
//
//	slug              Assigned once by a trigger on insert and never
//	                  regenerated. A renamed organiser keeps their original
//	                  URL, which is what makes a link shared in March still
//	                  resolve in October. Changing it would need a redirect
//	                  table to stay honest.
//	verified          Set by admin review, not by the subject of it.
//	kyc_level         Set by the KYC flow.
//	status            Set by admin.
//	commission_rate   Commercial terms.
//	description       Belongs to the PUBLIC page, so it lives in
//	                  UpdatePublicProfile. Splitting it would mean two writers
//	                  for one column.
//
// A sparse update: a nil pointer leaves the column alone. Sending an empty
// string for website_url clears it, and that has to stay distinguishable from
// not sending the key — which is the whole reason the DTO uses pointers.
func UpdateOrganizerAccount(
	pool *pgxpool.Pool,
	userID string,
	dto *models.UpdateOrganizerAccountDTO,
) (*models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sets := []string{}
	args := []interface{}{}

	if dto.BusinessName != nil {
		args = append(args, strings.TrimSpace(*dto.BusinessName))
		sets = append(sets, fmt.Sprintf("business_name = $%d", len(args)))
	}
	if dto.BusinessEmail != nil {
		args = append(args, strings.TrimSpace(*dto.BusinessEmail))
		sets = append(sets, fmt.Sprintf("business_email = $%d", len(args)))
	}
	if dto.BusinessPhone != nil {
		args = append(args, strings.TrimSpace(*dto.BusinessPhone))
		sets = append(sets, fmt.Sprintf("business_phone = $%d", len(args)))
	}
	if dto.WebsiteURL != nil {
		w := strings.TrimSpace(*dto.WebsiteURL)
		if w == "" {
			sets = append(sets, "website_url = NULL")
		} else {
			args = append(args, w)
			sets = append(sets, fmt.Sprintf("website_url = $%d", len(args)))
		}
	}

	if len(sets) == 0 {
		return GetOrganizerByUserID(pool, userID)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, userID)

	tag, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE organizers SET %s WHERE user_id = $%d`,
		strings.Join(sets, ", "), len(args),
	), args...)
	if err != nil {
		return nil, fmt.Errorf("UpdateOrganizerAccount: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrOrganizerNotFound
	}

	return GetOrganizerByUserID(pool, userID)
}