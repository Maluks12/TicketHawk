package handlers

import (
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Campaigns ─────────────────────────────────────────────────────────────────

// CreateCampaignHandler creates a boost campaign for a published event.
//
// Pre-paid wallet model: the campaign starts 'active' immediately but only
// serves while the organiser's ad wallet has a positive balance. No card is
// stored on the ad account.
//
//	POST /organiser/events/:id/campaigns
//	Body: { "ad_account_id": "...", "daily_budget": 200, "end_date": "2026-12-31" }
func CreateCampaignHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")
		eventID := c.Param("id")

		var dto models.CreateCampaignDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		campaign, err := repository.CreateCampaign(pool, eventID, orgID, &dto)
		if err != nil {
			switch err {
			case repository.ErrCampaignUnauthorized:
				c.JSON(http.StatusForbidden, gin.H{"error": "You do not own this event"})
			case repository.ErrAdAccountUnauthorized:
				c.JSON(http.StatusForbidden, gin.H{"error": "That ad account does not belong to you"})
			case repository.ErrEventAlreadyBoosted:
				c.JSON(http.StatusConflict, gin.H{
					"error": "This event already has an active campaign",
					"note":  "Pause or end the existing campaign before creating a new one",
				})
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			}
			return
		}

		// Surface the wallet so the UI can prompt a top-up if the balance is low.
		wallet, _ := repository.GetOrCreateAdWallet(pool, orgID)
		balance := 0.0
		if wallet != nil {
			balance = wallet.Balance
		}

		c.JSON(http.StatusCreated, gin.H{
			"campaign":       campaign,
			"message":        "Campaign created.",
			"wallet_balance": balance,
			"serving":        balance > 0,
			"note": func() string {
				if balance > 0 {
					return "Campaign is active and serving impressions, funded by your ad wallet."
				}
				return "Campaign is active but will not serve until you top up your ad wallet."
			}(),
			"pricing": gin.H{
				"cpm_rate":             campaign.CPMRate,
				"cost_per_impression":  campaign.CostPerImpression(),
				"daily_impression_cap": campaign.DailyImpressionCap(),
				"currency":             models.DefaultCurrency,
				"minimum_daily_budget": 200,
			},
		})
	}
}

// GetMyCampaignsHandler returns all campaigns for the authenticated organiser.
//
//	GET /organiser/campaigns
func GetMyCampaignsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")
		campaigns, err := repository.GetOrganizerCampaigns(pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch campaigns"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"campaigns": campaigns, "count": len(campaigns)})
	}
}

// GetCampaignHandler returns a single campaign with today's live metrics.
//
//	GET /organiser/campaigns/:id
func GetCampaignHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")
		campaignID := c.Param("id")
		campaign, err := repository.GetCampaign(pool, campaignID, orgID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Campaign not found"})
			return
		}
		c.JSON(http.StatusOK, campaign)
	}
}

// UpdateCampaignHandler lets organisers pause/resume, change budget, or extend end date.
//
//	PATCH /organiser/campaigns/:id
func UpdateCampaignHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")
		campaignID := c.Param("id")

		var dto models.UpdateCampaignDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		campaign, err := repository.UpdateCampaign(pool, campaignID, orgID, &dto)
		if err != nil {
			switch err {
			case repository.ErrCampaignNotFound:
				c.JSON(http.StatusNotFound, gin.H{"error": "Campaign not found"})
			case repository.ErrCampaignUnauthorized:
				c.JSON(http.StatusForbidden, gin.H{"error": "Not your campaign"})
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"campaign": campaign, "message": "Campaign updated"})
	}
}

// EndCampaignHandler permanently ends a campaign.
//
//	DELETE /organiser/campaigns/:id
func EndCampaignHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")
		campaignID := c.Param("id")

		if err := repository.EndCampaign(pool, campaignID, orgID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Campaign not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Campaign ended"})
	}
}

// ── Admin ─────────────────────────────────────────────────────────────────────

// AdminGetCampaignsHandler returns all campaigns across all organisers.
//
//	GET /admin/campaigns?status=active
func AdminGetCampaignsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		campaigns, err := repository.GetAllCampaigns(pool, c.Query("status"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch campaigns"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"campaigns": campaigns, "count": len(campaigns)})
	}
}