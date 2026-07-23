package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
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
func PaystackWebhookHandler(
	paymentService *services.PaymentService,
	refundService *services.RefundService,
	payoutService *services.PayoutService,
) gin.HandlerFunc {
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
				Reference string  `json:"reference"`
				Status    string  `json:"status"`
				Amount    int64   `json:"amount"`  // minor units
				Reason    string  `json:"reason"`
				Message   string  `json:"message"`
				ID        int64   `json:"id"`
				// refund.* nests the original charge under transaction
				Transaction struct {
					Reference string `json:"reference"`
				} `json:"transaction"`
				// charge.dispute.* nests it the same way
				TransactionReference string `json:"transaction_reference"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &evt); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		// Transfer lifecycle: a transfer to an organiser can be accepted by the
		// API and then fail asynchronously (reversed, invalid account, bank
		// rejection). Without this the payout row sat in 'processing' forever
		// and nobody learned the organiser was never paid.
		switch evt.Event {
		case "transfer.success", "transfer.failed", "transfer.reversed":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			status := "failed"
			if evt.Event == "transfer.success" {
				status = "success"
			}
			reason := evt.Data.Reason
			if reason == "" {
				reason = evt.Data.Message
			}
			if err := payoutService.ApplyTransferWebhook(ctx, evt.Data.Reference, status, reason); err != nil {
				log.Error().Err(err).Str("reference", evt.Data.Reference).
					Str("event", evt.Event).Msg("paystack webhook: transfer update failed")
			}
			c.JSON(http.StatusOK, gin.H{"received": true})
			return

		// A refund issued from the Paystack dashboard never touched our
		// database before this. The organiser would then be settled in full on
		// money the buyer already had back.
		case "refund.processed", "refund.failed":
			if evt.Event == "refund.failed" {
				log.Warn().Str("reference", evt.Data.Transaction.Reference).
					Msg("paystack webhook: refund FAILED at gateway — buyer not repaid")
				c.JSON(http.StatusOK, gin.H{"received": true})
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			ref := evt.Data.Transaction.Reference
			if ref == "" {
				ref = evt.Data.Reference
			}
			if err := refundService.SyncGatewayRefund(ctx, ref, float64(evt.Data.Amount)/100.0); err != nil {
				log.Error().Err(err).Str("reference", ref).
					Msg("paystack webhook: refund sync failed")
			}
			c.JSON(http.StatusOK, gin.H{"received": true})
			return

		// Chargebacks. Recorded, not acted on: a dispute being opened is not yet
		// a loss, and auto-clawing back would punish organisers who go on to win.
		case "charge.dispute.create", "charge.dispute.remind", "charge.dispute.resolve":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			ref := evt.Data.TransactionReference
			if ref == "" {
				ref = evt.Data.Transaction.Reference
			}
			if ref == "" {
				ref = evt.Data.Reference
			}
			if err := refundService.RecordDispute(ctx, ref,
				strconv.FormatInt(evt.Data.ID, 10), evt.Event, evt.Data.Reason,
				float64(evt.Data.Amount)/100.0, body); err != nil {
				log.Error().Err(err).Str("reference", ref).
					Msg("paystack webhook: dispute not recorded")
			}
			log.Warn().Str("reference", ref).Str("event", evt.Event).
				Float64("amount", float64(evt.Data.Amount)/100.0).
				Msg("paystack webhook: DISPUTE — review before the event settles")
			c.JSON(http.StatusOK, gin.H{"received": true})
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