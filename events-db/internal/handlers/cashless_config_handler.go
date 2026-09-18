package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Offline spend configuration (organiser) ───────────────────────────────────
//
// Until these existed, every safe default in Phase 4 assumed a row nothing
// could create — enabling offline spend meant raw SQL on production.
//
// ORGANISER-OWNED, not staff. Turning offline spend on decides that vendor
// devices may hold a forgery-capable key and that wallets may go negative. That
// is a commercial risk decision, and a scanner supervisor should not be able to
// make it — which is why these use the event's organizer_id rather than
// requireStaffManager.

// requireEventOwner resolves the event and confirms the caller owns it.
// Mirrors the check in organiser_event_edit_handler.go, where organizer_id is
// the user id directly.
func requireEventOwner(c *gin.Context, ctx context.Context, pool *pgxpool.Pool) string {
	userID := c.MustGet("user_id").(string)
	eventID := c.Param("id")

	var found string
	err := pool.QueryRow(ctx,
		`SELECT id FROM events WHERE id = $1 AND organizer_id = $2`,
		eventID, userID).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return ""
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load event"})
		return ""
	}
	return found
}

// GetOfflineConfigHandler returns the current configuration, or the documented
// defaults for an event that has never been set up.
//
//	GET /v1/organiser/events/:id/cashless/offline-config
func GetOfflineConfigHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireEventOwner(c, ctx, pool)
		if eventID == "" {
			return
		}

		cfg, err := repository.GetOfflineConfig(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load configuration"})
			return
		}

		resp := gin.H{"config": cfg}
		if !cfg.Configured {
			resp["note"] = "Offline spend has never been set up for this event. " +
				"The values shown are suggested defaults and nothing is enabled yet."
		}
		c.JSON(http.StatusOK, resp)
	}
}

// UpdateOfflineConfigHandler stores a configuration.
//
//	PUT /v1/organiser/events/:id/cashless/offline-config
//	{ "enabled": true, "max_offline_taps": 3, "max_offline_spend": 50.00,
//	  "max_distinct_vendors": 2, "unknown_tag_allowance": 5.00,
//	  "decay_steps": [{"after_minutes":10,"max_spend_cents":2500},
//	                  {"after_minutes":20,"max_spend_cents":0}] }
//
// Validation returns EVERY problem at once. An organiser fixing one error at a
// time through a form is an organiser who gives up and asks for the caps to be
// raised wholesale.
func UpdateOfflineConfigHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireEventOwner(c, ctx, pool)
		if eventID == "" {
			return
		}
		userID := c.MustGet("user_id").(string)

		var in struct {
			Enabled             bool                   `json:"enabled"`
			MaxOfflineTaps      int                    `json:"max_offline_taps"`
			MaxOfflineSpend     float64                `json:"max_offline_spend"`
			MaxDistinctVendors  int                    `json:"max_distinct_vendors"`
			DecaySteps          []repository.DecayStep `json:"decay_steps"`
			UnknownTagAllowance float64                `json:"unknown_tag_allowance"`
			MaxRecoveryAttempts int                    `json:"max_recovery_attempts"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		spendCents, err := repository.ToCents(in.MaxOfflineSpend)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid max_offline_spend"})
			return
		}
		allowCents, err := repository.ToCents(in.UnknownTagAllowance)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid unknown_tag_allowance"})
			return
		}

		if problems := repository.ValidateOfflinePolicy(repository.OfflinePolicy{
			Enabled:                  in.Enabled,
			MaxOfflineTaps:           in.MaxOfflineTaps,
			MaxOfflineSpendCents:     spendCents,
			MaxDistinctVendors:       in.MaxDistinctVendors,
			DecaySteps:               in.DecaySteps,
			UnknownTagAllowanceCents: allowCents,
		}); len(problems) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "Configuration would weaken the offline spend controls",
				"problems": problems,
			})
			return
		}

		attempts := in.MaxRecoveryAttempts
		if attempts < 0 || attempts > 10 {
			attempts = 3
		}

		if err := repository.UpsertOfflineConfig(ctx, pool, eventID, repository.OfflineConfig{
			Enabled:             in.Enabled,
			MaxOfflineTaps:      in.MaxOfflineTaps,
			MaxOfflineSpend:     in.MaxOfflineSpend,
			MaxDistinctVendors:  in.MaxDistinctVendors,
			DecaySteps:          in.DecaySteps,
			UnknownTagAllowance: in.UnknownTagAllowance,
			MaxRecoveryAttempts: attempts,
		}, userID); err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("offline config: upsert failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save configuration"})
			return
		}

		// Logged at warn on enable, not info. Turning this on decides that
		// vendor devices may hold a forgery-capable key — worth finding in a
		// log later without grep gymnastics.
		if in.Enabled {
			log.Warn().Str("event_id", eventID).Str("by", userID).
				Int64("device_cap_cents", spendCents).Int("taps", in.MaxOfflineTaps).
				Msg("offline spend ENABLED for event")
		} else {
			log.Info().Str("event_id", eventID).Str("by", userID).
				Msg("offline spend configuration saved (disabled)")
		}

		cfg, _ := repository.GetOfflineConfig(ctx, pool, eventID)
		resp := gin.H{"config": cfg}
		if in.Enabled {
			resp["warning"] = "Offline spend is now enabled. Tills that pass attestation " +
				"will receive this event's tag key, and wallets can go negative up to the " +
				"caps above. Disable it from this endpoint at any time."
		}
		c.JSON(http.StatusOK, resp)
	}
}

// DisableOfflineSpendHandler is the panic button.
//
//	POST /v1/organiser/events/:id/cashless/offline-config/disable
//	{ "reason": "till reported stolen" }
//
// Turns the policy off, drops every till out of offline mode, and revokes every
// live key grant — in one transaction, because doing them separately leaves a
// window where a device still holds a usable key while the policy says off.
func DisableOfflineSpendHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		eventID := requireEventOwner(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			Reason string `json:"reason" binding:"required,max=200"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.DisableOfflineSpend(ctx, pool, eventID, in.Reason); err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("offline config: disable failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable offline spend"})
			return
		}

		log.Warn().Str("event_id", eventID).Str("reason", in.Reason).
			Msg("offline spend DISABLED and all key grants revoked")

		c.JSON(http.StatusOK, gin.H{
			"message": "Offline spend disabled and every key grant revoked.",
			// Said plainly rather than left as a surprise: this cannot reach a
			// device that is already offline. Nothing can.
			"note": "Tills that are currently offline will keep approving until they " +
				"next reach the network. Their queued sales will still sync, and every " +
				"one will be flagged for review.",
		})
	}
}