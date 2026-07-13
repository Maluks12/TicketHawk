package handlers

import (
	"net/http"
	"strings"
	"time"

	"events-db/internal/cache"
	"events-db/internal/config"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Admin event handover (Path 4)
//
//   PUT /v1/admin/events/:id/organizer   — transfer an existing event
//
// plus a shared helper the admin CREATE handler calls when the body carries
// organizer_email, so create-for-organiser and transfer-later behave
// identically: invite-if-missing, auto-approve, fee marked settled
// (admin-promoted standing), handover email sent immediately.
// ─────────────────────────────────────────────────────────────────────────────

// Handover invites aren't a 30-minute "I forgot my password" flow — the
// organiser may not read the email today. One week to claim the account.
const handoverTokenTTL = 7 * 24 * time.Hour

type transferEventRequest struct {
	OrganizerEmail string `json:"organizer_email" binding:"required,email,max=255"`
	// Optional: seeds the organizers profile for brand-new invites. Existing
	// profiles are never renamed. Defaults to the email's local part.
	BusinessName string `json:"business_name" binding:"omitempty,max=255"`
}

// handoverEventToOrganizer performs the full handover for an already-loaded
// event: ensure the account, transfer ownership, mint a 7-day set-password
// token, and send the handover email (best-effort — a SendGrid failure never
// fails the transfer; ownership is already correct in the DB and the
// organiser can use "forgot password" on the dashboard as a fallback).
func handoverEventToOrganizer(
	pool *pgxpool.Pool,
	emailService *services.EmailService,
	cfg *config.Config,
	event *repository.EventOwnershipInfo,
	organizerEmail, businessName string,
) (gin.H, error) {
	organizerEmail = strings.ToLower(strings.TrimSpace(organizerEmail))
	if businessName = strings.TrimSpace(businessName); businessName == "" {
		// Sensible default for invites: the email's local part. They can set
		// their real business name from the dashboard.
		businessName = strings.SplitN(organizerEmail, "@", 2)[0]
	}

	// Unusable random placeholder password for invited accounts. Never
	// disclosed anywhere — the set-password link is the only way in.
	rawPlaceholder, _, err := utils.GenerateResetToken()
	if err != nil {
		return nil, err
	}
	placeholderHash, err := utils.HashPassword(rawPlaceholder)
	if err != nil {
		return nil, err
	}

	account, err := repository.EnsureOrganizerAccount(pool, organizerEmail, businessName, placeholderHash)
	if err != nil {
		return nil, err
	}

	if err := repository.TransferEventOwnership(pool, event.ID, account.UserID); err != nil {
		return nil, err
	}

	// Set-password token so the handover email works for BOTH cases: a fresh
	// invite creating their password, and an existing organiser who forgot
	// theirs. Reuses the standard password_resets flow (hashed at rest,
	// single-use, atomic consume).
	rawToken, hashedToken, err := utils.GenerateResetToken()
	if err != nil {
		return nil, err
	}
	if err := repository.CreatePasswordReset(pool, account.UserID, hashedToken, time.Now().Add(handoverTokenTTL)); err != nil {
		return nil, err
	}

	dashboardURL := cfg.OrganizerDashboardURL
	resetLink := dashboardURL + "/reset-password?token=" + rawToken

	emailSent := true
	if err := emailService.SendOrganizerHandover(
		organizerEmail, businessName, event.Title, resetLink, dashboardURL, account.NewAccount,
	); err != nil {
		emailSent = false
		log.Warn().Err(err).
			Str("event_id", event.ID).
			Str("organizer_email", organizerEmail).
			Msg("event handover: email failed (ownership already transferred; organiser can use forgot-password)")
	}

	return gin.H{
		"event_id":       event.ID,
		"event_title":    event.Title,
		"previous_owner": event.OrganizerID,
		"new_organizer": gin.H{
			"user_id":         account.UserID,
			"email":           account.Email,
			"account_created": account.NewAccount,
		},
		"email_sent": emailSent,
	}, nil
}

// AdminTransferEventHandler hands an existing event to an organiser.
//
//	PUT /v1/admin/events/:id/organizer
//	body: { "organizer_email": "org@example.com", "business_name": "Optional Co" }
func AdminTransferEventHandler(pool *pgxpool.Pool, ec *cache.EventCache, emailService *services.EmailService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")
		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		var req transferEventRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		event, err := repository.GetEventOwnership(pool, eventID)
		if err != nil {
			if err == repository.ErrEventNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "failed to load event")
			return
		}

		result, err := handoverEventToOrganizer(pool, emailService, cfg, event, req.OrganizerEmail, req.BusinessName)
		if err != nil {
			utils.InternalError(c, err, "failed to transfer event")
			return
		}

		// Ownership drives feed queries and dashboards — drop stale caches.
		if ec != nil {
			ec.BustOnPublish(c.Request.Context())
		}

		result["message"] = "Event transferred. The organiser has been emailed a link to set their password and access their dashboard."
		c.JSON(http.StatusOK, result)
	}
}