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

// Admin payout endpoints — mounted under the existing /v1/admin group.

// AdminListOrganizerPayoutProfilesHandler lists every organizer with their
// payout profile and instant-tier eligibility, for the admin Organizers page.
//
//	GET /v1/admin/organizers/payouts
func AdminListOrganizerPayoutProfilesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	type row struct {
		models.PayoutProfile
		BusinessEmail string                         `json:"business_email"`
		Status        string                         `json:"status"`
		Eligibility   *models.InstantTierEligibility `json:"eligibility,omitempty"`
	}
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		rows, err := pool.Query(ctx, `
			SELECT id, business_name, business_email, status::text, payout_tier, payout_enabled,
			       paystack_subaccount_code, settlement_bank_name,
			       bank_account_name, bank_account_last_4, tier_upgraded_at
			FROM organizers
			ORDER BY created_at DESC
			LIMIT 500
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load organizers"})
			return
		}
		defer rows.Close()

		var out []row
		for rows.Next() {
			var r row
			var tier string
			var subCode *string
			if err := rows.Scan(&r.OrganizerID, &r.BusinessName, &r.BusinessEmail, &r.Status,
				&tier, &r.PayoutEnabled, &subCode, &r.SettlementBankName,
				&r.BankAccountName, &r.BankAccountLast4, &r.TierUpgradedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Scan failed"})
				return
			}
			r.PayoutTier = models.PayoutTier(tier)
			r.HasSubaccount = subCode != nil && *subCode != ""
			r.SubaccountCode = subCode // admins may see the ACCT code
			out = append(out, r)
		}

		// Eligibility per organizer. Fine at admin-page scale; add batching
		// only if the organizer list grows past a few hundred.
		for i := range out {
			if e, eErr := repository.CheckInstantEligibility(pool, ctx, out[i].OrganizerID); eErr == nil {
				out[i].Eligibility = e
			}
		}

		if out == nil {
			out = []row{}
		}
		c.JSON(http.StatusOK, gin.H{"organizers": out})
	}
}

// AdminSetOrganizerTierHandler promotes/demotes an organizer's payout tier.
// Promotion is allowed even when the eligibility checklist isn't fully green
// (admin override), but the response echoes eligibility so the UI can warn.
// The DB constraint still hard-blocks 'instant' without a subaccount.
//
//	PUT /v1/admin/organizers/:id/tier
func AdminSetOrganizerTierHandler(pool *pgxpool.Pool, payoutService *services.PayoutService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.Param("id")
		adminID, _ := c.Get("user_id")

		var dto models.AdminSetTierDTO
		if err := c.ShouldBindJSON(&dto); err != nil ||
			(dto.Tier != models.PayoutTierHeld && dto.Tier != models.PayoutTierInstant) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tier must be 'held' or 'instant'"})
			return
		}

		ctx := c.Request.Context()

		if dto.Tier == models.PayoutTierInstant {
			profile, pErr := repository.GetPayoutProfileByOrganizerID(pool, ctx, organizerID)
			if pErr != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Organizer not found"})
				return
			}
			if !profile.HasSubaccount || !profile.PayoutEnabled {
				c.JSON(http.StatusConflict, gin.H{"error": "Organizer has no verified bank details / subaccount — instant settlement is impossible without one"})
				return
			}

			// Auto-run the paid DHA ID check (bank ownership was checked at detail
			// entry). Full ID match + verified bank → auto-approve. Anything else
			// stops here with the reason, unless the admin explicitly overrides
			// with ?override=true (their judgement, logged).
			if c.Query("override") != "true" {
				check, cErr := payoutService.RunPromotionChecks(ctx, organizerID)
				if cErr != nil {
					log.Error().Err(cErr).Str("organizer_id", organizerID).Msg("admin: promotion check errored")
					c.JSON(http.StatusBadGateway, gin.H{"error": "Verification check failed to run", "detail": cErr.Error()})
					return
				}
				if !check.AutoApprove {
					c.JSON(http.StatusConflict, gin.H{
						"error":  "Automated verification did not clear this organizer for instant payouts.",
						"reason": check.Reason,
						"checks": check,
						"hint":   "Review the details and re-send with ?override=true to promote anyway.",
					})
					return
				}
			}
		}

		if err := repository.SetOrganizerTier(pool, ctx, organizerID, dto.Tier, adminID.(string)); err != nil {
			log.Error().Err(err).Str("organizer_id", organizerID).Msg("admin: set tier failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not update tier"})
			return
		}

		eligibility, _ := repository.CheckInstantEligibility(pool, ctx, organizerID)
		c.JSON(http.StatusOK, gin.H{
			"message":     "Payout tier updated",
			"tier":        dto.Tier,
			"eligibility": eligibility,
		})
	}
}

// AdminSetOrganizerPayoutDetailsHandler lets an admin enter bank details on
// behalf of an organizer — the Path 4 (admin-entered events) companion, so
// admin-managed organizers can be paid without ever logging in.
//
//	POST /v1/admin/organizers/:id/payout-details
func AdminSetOrganizerPayoutDetailsHandler(payoutService *services.PayoutService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.SetPayoutDetailsDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payout details: " + err.Error()})
			return
		}

		profile, err := payoutService.SetPayoutDetails(c.Request.Context(), c.Param("id"), &dto)
		if err != nil {
			log.Error().Err(err).Str("organizer_id", c.Param("id")).Msg("admin: set payout details failed")
			c.JSON(http.StatusBadGateway, gin.H{"error": "Payment provider rejected these bank details: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Payout details saved", "profile": profile})
	}
}

// AdminListPayoutsHandler returns the payout ledger, filterable by status.
//
//	GET /v1/admin/payouts?status=failed
func AdminListPayoutsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		payouts, err := repository.ListPayouts(pool, c.Request.Context(), c.Query("status"), c.Query("organizer_id"), 200)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load payouts"})
			return
		}
		if payouts == nil {
			payouts = []models.OrganizerPayout{}
		}
		c.JSON(http.StatusOK, gin.H{"payouts": payouts})
	}
}

// AdminRetryPayoutHandler re-runs the transfer pipeline immediately for one
// failed payout. Idempotency is preserved: the original transfer_reference is
// reused, so a retry can never double-pay.
//
//	POST /v1/admin/payouts/:id/retry
func AdminRetryPayoutHandler(pool *pgxpool.Pool, payoutService *services.PayoutService) gin.HandlerFunc {
	return func(c *gin.Context) {
		payoutID := c.Param("id")
		ctx := c.Request.Context()

		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM organizer_payouts WHERE id = $1`, payoutID).Scan(&status); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Payout not found"})
			return
		}
		if status == "paid" {
			c.JSON(http.StatusConflict, gin.H{"error": "Payout is already paid"})
			return
		}

		// Run the whole pipeline; the targeted row is pending/failed/stuck and
		// will be picked up, and any other payable rows ride along harmlessly.
		payoutService.ProcessPendingPayouts(ctx)

		var id, afterStatus, failureReason, transferCode string
		err := pool.QueryRow(ctx, `
			SELECT id, status, COALESCE(failure_reason, ''), COALESCE(paystack_transfer_code, '')
			FROM organizer_payouts WHERE id = $1
		`, payoutID).Scan(&id, &afterStatus, &failureReason, &transferCode)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "Retry triggered"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "Retry triggered",
			"payout": gin.H{
				"id": id, "status": afterStatus,
				"failure_reason": failureReason, "paystack_transfer_code": transferCode,
			},
		})
	}
}