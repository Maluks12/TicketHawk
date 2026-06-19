package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// PaystackWebhookHandler handles all Paystack events — ticket/cart/merch
// payments, listing fees, and ad wallet top-ups. It verifies the HMAC-SHA512
// signature, re-fetches the authoritative status from Paystack (never trusting
// the webhook body), and applies it via the shared ApplyVerifiedPayment path,
// which routes by reference prefix (topup_ / lf_ / bk_ / cart_).
//
//	POST /webhooks/paystack
func PaystackWebhookHandler(paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unreadable body"})
			return
		}

		// Verify signature (constant-time HMAC-SHA512 over the raw body).
		if !paymentService.Paystack().VerifySignature(body, c.GetHeader("x-paystack-signature")) {
			log.Warn().Str("ip", c.ClientIP()).Msg("paystack webhook: invalid signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}

		var evt struct {
			Event string `json:"event"`
			Data  struct {
				Reference string `json:"reference"`
				Status    string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &evt); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		// We only act on charge-lifecycle events; acknowledge everything else.
		switch evt.Event {
		case "charge.success", "charge.failed", "charge.abandoned":
		default:
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}

		if evt.Data.Reference == "" {
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		// Re-fetch the truth from Paystack rather than trusting the webhook body.
		verified, err := paymentService.Paystack().Verify(ctx, evt.Data.Reference)
		if err != nil {
			log.Error().Err(err).Str("reference", evt.Data.Reference).
				Msg("paystack webhook: verify failed")
			// 200 so Paystack stops retrying; a reconciliation job can re-check.
			c.JSON(http.StatusOK, gin.H{"received": true, "warning": "verify failed"})
			return
		}

		if err := paymentService.ApplyVerifiedPayment(ctx, verified); err != nil {
			log.Error().Err(err).Str("reference", evt.Data.Reference).
				Msg("paystack webhook: apply failed")
			c.JSON(http.StatusOK, gin.H{"received": true, "warning": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"received": true})
	}
}