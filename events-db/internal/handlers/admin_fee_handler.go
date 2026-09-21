package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Per-organiser fee arrangements — MASTER ADMIN ONLY ───────────────────────
//
// Changing what an organiser pays changes where ticket money goes, so both the
// read and the write sit behind is_master_admin rather than a console
// capability: no value of staff_role reaches them. See migration 000093.

// RequireMasterAdminForFees gates the fee routes.
//
// Deliberately NOT RequireMasterAdmin from identity_document_handler.go: that
// one writes denials to kyc_document_access_log and answers "identity documents
// require master admin access", both wrong for this screen. Denials here go to
// staff_audit_log like every other console denial.
func RequireMasterAdminForFees(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var isMaster bool
		var email string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(is_master_admin, FALSE), email FROM users WHERE id = $1`,
			userID,
		).Scan(&isMaster, &email)
		if err != nil {
			// Fail closed.
			log.Error().Err(err).Msg("fees: master admin check failed")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}

		if !isMaster {
			// Captured here, on the request goroutine — gin recycles the
			// context once the chain returns (see auditDenied in middleware).
			path := c.Request.URL.Path
			ip := c.ClientIP()
			ua := c.Request.UserAgent()
			go func() {
				wctx, wcancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer wcancel()
				_, werr := pool.Exec(wctx, `
					INSERT INTO staff_audit_log
						(actor_id, actor_email, actor_role, action, allowed, detail, ip, user_agent)
					VALUES ($1, $2, (SELECT staff_role FROM users WHERE id = $1),
					        'organizer.fees', FALSE,
					        jsonb_build_object('reason', 'not master admin', 'path', $3::text),
					        NULLIF($4, '')::inet, $5)
				`, userID, email, path, ip, ua)
				if werr != nil {
					log.Warn().Err(werr).Msg("fees: denied-audit write failed")
				}
			}()
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "organiser fee arrangements require master admin access",
			})
			return
		}

		c.Set("actor_email", email)
		c.Next()
	}
}

// AdminListOrganizerFeesHandler lists every organiser with their fee terms,
// plus the platform defaults and limits the screen should use.
//
//	GET /v1/admin/fees/organizers
func AdminListOrganizerFeesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		rows, err := repository.ListOrganizerFees(ctx, pool)
		if err != nil {
			log.Error().Err(err).Msg("admin: list organiser fees failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load organiser fees"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"defaults":   repository.CurrentFeeDefaults(),
			"organizers": rows,
		})
	}
}

// adminSetFeesDTO is the complete intended state. A null (or omitted) field
// means "use the platform default" and clears any existing override.
type adminSetFeesDTO struct {
	CommissionPct *float64 `json:"commission_pct"`
	ServiceFee    *float64 `json:"service_fee"`
	Note          string   `json:"note"`
}

// AdminSetOrganizerFeesHandler sets or clears an organiser's fee arrangement.
// Only bookings created after the change are priced on the new terms.
//
//	PUT /v1/admin/fees/organizers/:id
//	{ "commission_pct": 4.5, "service_fee": 5, "note": "3-event deal, signed 2026-09-20" }
//	{ "commission_pct": null, "service_fee": null, "note": "" }   ← back to defaults
func AdminSetOrganizerFeesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.Param("id")
		if _, err := uuid.Parse(organizerID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organiser id"})
			return
		}

		var dto adminSetFeesDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)
		emailVal, _ := c.Get("actor_email")
		email, _ := emailVal.(string)

		actor := repository.AuditActor{
			UserID:    userIDStr,
			Email:     email,
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		}
		in := repository.FeeOverrideInput{
			CommissionPct: dto.CommissionPct,
			ServiceFee:    dto.ServiceFee,
			Note:          dto.Note,
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		before, after, err := repository.UpdateOrganizerFees(ctx, pool, organizerID, in, actor)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrFeeOrganizerNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "Organiser not found"})
			case errors.Is(err, repository.ErrFeeCommissionRange),
				errors.Is(err, repository.ErrFeeServiceFeeRange),
				errors.Is(err, repository.ErrFeeNoteRequired),
				errors.Is(err, repository.ErrFeeNoteTooLong):
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			default:
				log.Error().Err(err).Str("organizer_id", organizerID).Msg("admin: set organiser fees failed")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not update fees"})
			}
			return
		}

		log.Info().
			Str("organizer_id", organizerID).
			Str("actor", email).
			Float64("commission_before", before.EffectiveCommissionPct).
			Float64("commission_after", after.EffectiveCommissionPct).
			Float64("service_fee_before", before.EffectiveServiceFee).
			Float64("service_fee_after", after.EffectiveServiceFee).
			Msg("admin: organiser fees updated")

		c.JSON(http.StatusOK, gin.H{
			"message":   "Fees updated. New bookings for this organiser use these terms.",
			"organizer": after,
		})
	}
}