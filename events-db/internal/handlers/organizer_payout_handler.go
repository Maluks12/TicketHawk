package handlers

import (
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Organizer-facing payout endpoints. All mounted under the authenticated
// /v1/organiser group (AuthMiddleware + OrganizerOnlyMiddleware), so "user_id"
// is present and the user owns an organizer row. The user→organizers.id
// translation reuses resolveOrganizerID from ad_account_handler.go.

// GetPayoutBanksHandler returns the ZA settlement bank list for the bank
// picker. Cached in-process for 24h inside the gateway.
//
//	GET /v1/organiser/payouts/banks
func GetPayoutBanksHandler(payoutService *services.PayoutService) gin.HandlerFunc {
	return func(c *gin.Context) {
		banks, err := payoutService.Gateway().ListBanks(c.Request.Context())
		if err != nil {
			log.Error().Err(err).Msg("payouts: listing banks")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Could not load bank list, please try again"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"banks": banks})
	}
}

// SetPayoutDetailsHandler creates/updates the organizer's Paystack subaccount
// and transfer recipient from submitted bank details, enabling payouts.
//
//	POST /v1/organiser/payouts/details
func SetPayoutDetailsHandler(pool *pgxpool.Pool, payoutService *services.PayoutService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}

		var dto models.SetPayoutDetailsDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payout details: " + err.Error()})
			return
		}

		profile, err := payoutService.SetPayoutDetails(c.Request.Context(), organizerID, &dto)
		if err != nil {
			log.Error().Err(err).Str("organizer_id", organizerID).Msg("payouts: set details failed")
			c.JSON(http.StatusBadGateway, gin.H{"error": "Could not verify these bank details with our payment provider. Please check the account number and bank, then try again."})
			return
		}

		profile.SubaccountCode = nil // internal identifier, not for organizer UI
		c.JSON(http.StatusOK, gin.H{"message": "Payout details saved", "profile": profile})
	}
}

// GetPayoutProfileHandler returns the organizer's payout setup, tier, and
// their progress toward instant-tier eligibility (the checklist).
//
//	GET /v1/organiser/payouts/profile
func GetPayoutProfileHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}

		profile, err := repository.GetPayoutProfileByOrganizerID(pool, c.Request.Context(), organizerID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load payout profile"})
			return
		}
		profile.SubaccountCode = nil

		eligibility, err := repository.CheckInstantEligibility(pool, c.Request.Context(), organizerID)
		if err != nil {
			log.Warn().Err(err).Str("organizer_id", organizerID).Msg("payouts: eligibility check failed")
			eligibility = nil // profile still useful without it
		}

		c.JSON(http.StatusOK, gin.H{
			"profile":                  profile,
			"instant_tier_eligibility": eligibility,
		})
	}
}

// GetPayoutHistoryHandler returns the organizer's payout ledger rows.
//
//	GET /v1/organiser/payouts/history
func GetPayoutHistoryHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}

		payouts, err := repository.ListPayouts(pool, c.Request.Context(), c.Query("status"), organizerID, 100)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load payout history"})
			return
		}
		if payouts == nil {
			payouts = []models.OrganizerPayout{}
		}
		c.JSON(http.StatusOK, gin.H{"payouts": payouts})
	}
}