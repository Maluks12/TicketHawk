package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Wallet ────────────────────────────────────────────────────────────────────

// GetWalletHandler returns the organiser's ad wallet balance and lifetime totals.
//
//	GET /organiser/ad-wallet
func GetWalletHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")
		wallet, err := repository.GetOrCreateWallet(pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch wallet"})
			return
		}
		c.JSON(http.StatusOK, wallet)
	}
}

// TopupWalletHandler creates a Stitch payment intent for an ad wallet top-up.
//
//	POST /organiser/ad-wallet/topup
//	Body: { "amount": 500 }
//
// Flow:
//  1. Organiser POSTs amount → we create a Stitch payment intent
//  2. Frontend redirects to the Stitch hosted page
//  3. Stitch POSTs to /webhooks/stitch-ads on completion
//  4. AdTopupWebhookHandler calls ConfirmTopup → wallet credited → campaigns resume
func TopupWalletHandler(pool *pgxpool.Pool, paymentSvc *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")

		var dto models.TopupWalletDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		clientToken, requestID, err := paymentSvc.CreateAdTopupIntent(orgID, dto.Amount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create payment — please try again"})
			return
		}

		topup, err := repository.CreateTopup(pool, orgID, dto.Amount, requestID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record topup"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"topup_id":        topup.ID,
			"reference":       topup.ReferenceNumber,
			"amount":          dto.Amount,
			"currency":        "ZAR",
			"client_token":    clientToken,
			"payment_request": requestID,
			"status":          "pending",
			"message":         "Complete payment to credit your ad wallet",
		})
	}
}

// ── Campaigns ─────────────────────────────────────────────────────────────────

// CreateCampaignHandler creates a boost campaign for an event the organiser owns.
//
//	POST /organiser/events/:id/campaigns
//	Body: { "daily_budget": 200, "end_date": "2026-06-30" }
//
// Campaign starts as 'active' if wallet balance >= daily_budget; 'draft' otherwise.
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
				c.JSON(http.StatusForbidden, gin.H{"error": "you do not own this event"})
			case repository.ErrEventAlreadyBoosted:
				c.JSON(http.StatusConflict, gin.H{
					"error": "this event already has a campaign",
					"note":  "pause or end the existing campaign before creating a new one",
				})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create campaign"})
			}
			return
		}

		msg := "Campaign is active and serving impressions"
		if campaign.Status == models.CampaignDraft {
			msg = "Campaign created — top up your wallet to R" +
				fmt.Sprintf("%.0f", campaign.DailyBudget) + " or more to activate"
		}

		c.JSON(http.StatusCreated, gin.H{
			"campaign": campaign,
			"message":  msg,
			"pricing": gin.H{
				"cpm_rate":             campaign.CPMRate,
				"cost_per_impression":  campaign.CostPerImpression(),
				"daily_impression_cap": campaign.DailyImpressionCap(),
				"currency":             "ZAR",
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch campaigns"})
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
			c.JSON(http.StatusNotFound, gin.H{"error": "campaign not found"})
			return
		}
		c.JSON(http.StatusOK, campaign)
	}
}

// UpdateCampaignHandler lets organisers pause/resume a campaign, change
// daily budget, or extend the end date.
//
//	PATCH /organiser/campaigns/:id
//	Body examples:
//	  { "status": "paused" }
//	  { "status": "active" }
//	  { "daily_budget": 300 }
//	  { "end_date": "2026-07-31" }
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
				c.JSON(http.StatusNotFound, gin.H{"error": "campaign not found"})
			case repository.ErrCampaignUnauthorized:
				c.JSON(http.StatusForbidden, gin.H{"error": "not your campaign"})
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
			c.JSON(http.StatusNotFound, gin.H{"error": "campaign not found"})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch campaigns"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"campaigns": campaigns, "count": len(campaigns)})
	}
}

// ── Webhook ───────────────────────────────────────────────────────────────────

// AdTopupWebhookHandler handles Stitch confirmations for ad wallet top-ups.
// This is a separate endpoint from /webhooks/stitch so ad billing and ticket
// billing never share code paths.
//
//	POST /webhooks/stitch-ads
func AdTopupWebhookHandler(pool *pgxpool.Pool, webhookSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unreadable body"})
			return
		}

		// Verify HMAC signature — same pattern as StitchWebhookHandler
		sig := c.GetHeader("X-Stitch-Signature")
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}

		var payload struct {
			Data struct {
				Node struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"node"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}

		requestID := payload.Data.Node.ID
		status := payload.Data.Node.Status

		if status != "Completed" {
			pool.Exec(c.Request.Context(), `
				UPDATE ad_wallet_topups
				SET    status = 'failed'
				WHERE  stitch_payment_request_id = $1
				  AND  status = 'pending'
			`, requestID)
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}

		if _, err := repository.ConfirmTopup(pool, requestID); err != nil {
			// Already processed or not found — return 200 to stop Stitch retrying
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}

		c.JSON(http.StatusOK, gin.H{"received": true, "credited": true})
	}
}