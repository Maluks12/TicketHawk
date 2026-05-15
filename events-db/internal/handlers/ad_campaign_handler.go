package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Payment method ────────────────────────────────────────────────────────────

// RegisterPaymentMethodHandler stores the organiser's Stitch debit mandate.
//
// Flow:
//  1. Frontend guides the organiser through Stitch's mandate OAuth flow
//  2. Frontend receives the mandate ID from Stitch
//  3. Frontend POST here — mandate is stored, organiser is ready to run campaigns
//
// All future end-of-day billing debits happen automatically — no top-ups ever needed.
//
//	POST /organiser/ad-payment-method
//	Body: { "stitch_mandate_id": "man_abc123", "email": "org@example.com" }
func RegisterPaymentMethodHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("organizer_id")

		var dto models.RegisterPaymentMethodDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.RegisterPaymentMethod(pool, orgID, &dto); err != nil {
			if err == repository.ErrOrganizerNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Organizer not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register payment method"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Payment method registered successfully",
			"status":  "active",
			"note":    "Your campaigns will be billed automatically each morning for the previous day's ad spend. No top-ups required.",
		})
	}
}

// GetPaymentMethodStatusHandler returns whether the organiser has an active payment method.
//
//	GET /organiser/ad-payment-method
func GetPaymentMethodStatusHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("organizer_id")

		status, err := repository.GetPaymentMethodStatus(pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch payment method status"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      status,
			"has_mandate": status == "active",
			"billing":     "Automatic daily debit — billed each morning for previous day's actual spend",
		})
	}
}

// GetBillingHistoryHandler returns the organiser's past daily ad bills.
//
//	GET /organiser/ad-billing?limit=30
func GetBillingHistoryHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("organizer_id")
		limit := 30
		if l := c.Query("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 90 {
				limit = n
			}
		}

		records, err := repository.GetBillingHistory(pool, orgID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch billing history"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"billing_history": records,
			"count":           len(records),
		})
	}
}

// ── Campaigns ─────────────────────────────────────────────────────────────────

// CreateCampaignHandler creates a boost campaign for a published event.
//
// Post-pay: campaign starts as 'active' immediately — no wallet required.
// Organiser must have a Stitch debit mandate registered first.
//
//	POST /organiser/events/:id/campaigns
//	Body: { "daily_budget": 200, "end_date": "2026-12-31" }
func CreateCampaignHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("organizer_id")
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
			case repository.ErrEventAlreadyBoosted:
				c.JSON(http.StatusConflict, gin.H{
					"error": "This event already has an active campaign",
					"note":  "Pause or end the existing campaign before creating a new one",
				})
			case repository.ErrNoPaymentMethod:
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error": "No payment method on file",
					"note":  "Register a Stitch debit mandate via POST /organiser/ad-payment-method first",
				})
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			}
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"campaign": campaign,
			"message":  "Campaign is active and serving impressions",
			"billing":  "You will be billed automatically each morning for yesterday's actual spend",
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
		orgID := c.GetString("organizer_id")
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
		orgID := c.GetString("organizer_id")
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
		orgID := c.GetString("organizer_id")
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
		orgID := c.GetString("organizer_id")
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
//	GET /admin/campaigns?status=payment_failed
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

// ── Webhook ───────────────────────────────────────────────────────────────────

// AdBillingWebhookHandler handles Stitch confirmations for end-of-day ad billing debits.
//
// Called by Stitch when the nightly debit mandate charge completes or fails.
// On success: billing record marked completed, spend rows flagged billed, payment_failed campaigns resume.
// On failure: billing record marked failed, active campaigns paused as payment_failed.
//
//	POST /webhooks/stitch-ads
func AdBillingWebhookHandler(pool *pgxpool.Pool, webhookSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unreadable body"})
			return
		}

		// Verify HMAC signature — same pattern as the main StitchWebhookHandler.
		sig := c.GetHeader("X-Stitch-Signature")
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}

		var payload struct {
			Data struct {
				Node struct {
					ID     string `json:"id"`
					Status string `json:"status"`
					Reason string `json:"reason"`
				} `json:"node"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		requestID := payload.Data.Node.ID
		status := payload.Data.Node.Status

		switch status {
		case "Completed":
			if err := repository.ConfirmBilling(pool, requestID); err != nil {
				// Already processed — return 200 to stop Stitch retrying.
				c.JSON(http.StatusOK, gin.H{"received": true})
				return
			}
			c.JSON(http.StatusOK, gin.H{"received": true, "billed": true})

		case "Failed", "Cancelled", "Declined":
			reason := payload.Data.Node.Reason
			if reason == "" {
				reason = status
			}
			_ = repository.FailBilling(pool, requestID, reason)
			c.JSON(http.StatusOK, gin.H{"received": true, "failed": true})

		default:
			// Unknown/intermediate status — acknowledge to prevent Stitch retrying.
			c.JSON(http.StatusOK, gin.H{"received": true})
		}
	}
}