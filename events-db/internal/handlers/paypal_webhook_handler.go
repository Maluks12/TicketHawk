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

// PayPalWebhookHandler handles PayPal Orders v2 events for consumer ticket/merch
// payments. It verifies authenticity with PayPal's verify-webhook-signature API,
// resolves the order id, re-fetches/captures the order, and applies the result
// via the shared path (which looks the payment up by gateway_reference = order id).
//
//	POST /webhooks/paypal
func PayPalWebhookHandler(paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unreadable body"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		ok, err := paymentService.PayPal().VerifyWebhook(ctx, services.PayPalWebhookHeaders{
			AuthAlgo:         c.GetHeader("paypal-auth-algo"),
			CertURL:          c.GetHeader("paypal-cert-url"),
			TransmissionID:   c.GetHeader("paypal-transmission-id"),
			TransmissionSig:  c.GetHeader("paypal-transmission-sig"),
			TransmissionTime: c.GetHeader("paypal-transmission-time"),
		}, body)
		if err != nil || !ok {
			log.Warn().Err(err).Str("ip", c.ClientIP()).Msg("paypal webhook: verification failed")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid webhook"})
			return
		}

		var evt struct {
			EventType string `json:"event_type"`
			Resource  struct {
				ID                string `json:"id"`
				SupplementaryData struct {
					RelatedIDs struct {
						OrderID string `json:"order_id"`
					} `json:"related_ids"`
				} `json:"supplementary_data"`
			} `json:"resource"`
		}
		if err := json.Unmarshal(body, &evt); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		// Resolve the order id depending on event shape.
		orderID := evt.Resource.SupplementaryData.RelatedIDs.OrderID
		if orderID == "" {
			orderID = evt.Resource.ID // CHECKOUT.ORDER.* events carry the order id directly
		}

		switch evt.EventType {
		case "CHECKOUT.ORDER.APPROVED",
			"PAYMENT.CAPTURE.COMPLETED",
			"PAYMENT.CAPTURE.DENIED",
			"PAYMENT.CAPTURE.REFUNDED":
		default:
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}

		if orderID == "" {
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}

		// Verify (and capture if APPROVED) the order, then apply.
		verified, err := paymentService.PayPal().Verify(ctx, orderID)
		if err != nil {
			log.Error().Err(err).Str("order_id", orderID).Msg("paypal webhook: verify/capture failed")
			c.JSON(http.StatusOK, gin.H{"received": true, "warning": "verify failed"})
			return
		}

		if err := paymentService.ApplyVerifiedPayment(ctx, verified); err != nil {
			log.Error().Err(err).Str("order_id", orderID).Msg("paypal webhook: apply failed")
			c.JSON(http.StatusOK, gin.H{"received": true, "warning": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"received": true})
	}
}