package handlers

import (
	"errors"
	"events-db/internal/repository"
	"events-db/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GetListingFeeHandler returns the listing fee status for an event.
//
//	GET /organiser/events/:id/listing-fee
//
// Listing fee ONLY applies to free events (price = 0) with 20+ ticket capacity.
// Paid-ticket events are never charged and are always eligible for the home feed.
//
// Possible responses:
//
//	Paid event:
//	  { "is_free_event": false, "fee_required": false,
//	    "note": "Paid-ticket events are not charged a listing fee..." }
//
//	Free event, capacity < 20:
//	  { "is_free_event": true, "fee_required": false,
//	    "note": "Free events with fewer than 20 ticket capacity publish at no cost..." }
//
//	Free event, capacity >= 20, fee unpaid:
//	  { "is_free_event": true, "fee_required": true, "fee_paid": false,
//	    "fee_amount": 1000.00, "breakdown": "200 tickets × R5.00 = R1,000.00" }
//
//	Free event, capacity >= 20, fee paid:
//	  { "is_free_event": true, "fee_required": true, "fee_paid": true,
//	    "note": "Listing fee paid. Your event is eligible to appear in home-feed sections." }
func GetListingFeeHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		info, err := repository.GetListingFeeInfo(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve listing fee info"})
			return
		}

		resp := gin.H{
			"event_id":           info.EventID,
			"is_free_event":      info.IsFreeEvent,
			"effective_capacity": info.EffectiveCapacity,
			"fee_amount":         info.FeeAmount,
			"fee_paid":           info.FeePaid,
			"fee_required":       info.FeeRequired,
			"fee_per_ticket":     repository.ListingFeePerTicket,
			"threshold":          repository.ListingFeeThreshold,
		}

		switch {
		case !info.IsFreeEvent:
			// Paid-ticket event — listing fee never applies
			resp["note"] = "Paid-ticket events are not charged a listing fee and always " +
				"appear in home-feed sections once approved."

		case !info.FeeRequired:
			// Free event, capacity under threshold — publish free, no home feed
			resp["note"] = "Free events with fewer than 20 ticket capacity publish at no cost. " +
				"They are not shown in home-feed sections — run an ad campaign to gain algorithmic traction."

		case info.FeePaid:
			// Free event, fee already settled
			resp["breakdown"] = formatBreakdown(info.EffectiveCapacity, info.FeeAmount)
			resp["note"] = "Listing fee paid. Your free event is eligible to appear in home-feed sections."

		default:
			// Free event, fee due
			resp["breakdown"] = formatBreakdown(info.EffectiveCapacity, info.FeeAmount)
			resp["note"] = "Your free event has 20+ ticket capacity. Pay the listing fee (R5 per ticket, " +
				"absorbed by you as the organiser — ticket holders are not charged). " +
				"Call POST /organiser/events/:id/listing-fee/initiate to pay."
		}

		c.JSON(http.StatusOK, resp)
	}
}

// InitiateListingFeePaymentHandler creates a Stitch payment intent for the
// listing fee and returns the client token that the frontend uses to render
// the Stitch secure payment form.
//
//	POST /organiser/events/:id/listing-fee/initiate
//
// Only free events (price = 0) with capacity >= 20 can reach this endpoint
// successfully. Any other event type returns a descriptive 4xx error.
//
// Success response:
//
//	{
//	  "client_token":       "stitch_client_token",
//	  "payment_request_id": "stitch_request_id",
//	  "amount":             1000.00,
//	  "currency":           "ZAR",
//	  "breakdown":          "200 tickets × R5.00 = R1,000.00",
//	  "message":            "Complete payment using the client_token..."
//	}
func InitiateListingFeePaymentHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		// Recompute fee fresh — capacity or price may have changed since last check
		info, err := repository.GetListingFeeInfo(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute listing fee"})
			return
		}

		// Paid-ticket events are never subject to the listing fee
		if !info.IsFreeEvent {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Listing fee does not apply to paid-ticket events. " +
					"Only free events (ticket price = R0) with 20+ capacity are charged.",
			})
			return
		}

		// Free event but below threshold — no fee needed at all
		if !info.FeeRequired {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "No listing fee required. This free event has fewer than 20 ticket capacity " +
					"and can be published at no cost.",
			})
			return
		}

		// Already paid
		if info.FeePaid {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Listing fee has already been paid for this event.",
			})
			return
		}

		// Create the Stitch payment intent
		clientToken, requestID, err := paymentService.CreateListingFeeIntent(
			eventID, organizerID.(string), info.FeeAmount,
		)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Payment service temporarily unavailable. Please try again shortly.",
			})
			return
		}

		// Persist the request ID so the webhook can resolve it on confirmation
		if err := repository.SetListingFeePaymentRequest(
			pool, eventID, organizerID.(string), requestID, info.FeeAmount,
		); err != nil {
			switch {
			case errors.Is(err, repository.ErrListingFeeAlreadyPaid):
				c.JSON(http.StatusConflict, gin.H{"error": "Listing fee has already been paid."})
			case errors.Is(err, repository.ErrListingFeeNotApplicable):
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Listing fee does not apply to paid-ticket events.",
				})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record payment intent"})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"client_token":       clientToken,
			"payment_request_id": requestID,
			"amount":             info.FeeAmount,
			"currency":           "ZAR",
			"breakdown":          formatBreakdown(info.EffectiveCapacity, info.FeeAmount),
			"organiser_note":     "This fee is absorbed by you as the organiser. Ticket holders pay nothing extra.",
			"message": "Complete payment using the client_token in the Stitch SDK. " +
				"Your free event will surface in home-feed sections once payment is confirmed.",
		})
	}
}

// ── Formatting helpers ─────────────────────────────────────────────────────────

// formatBreakdown produces a human-readable fee breakdown string.
// e.g. "200 tickets × R5.00 = R1,000.00"
func formatBreakdown(capacity int, total float64) string {
	if capacity == 0 || total == 0 {
		return "No listing fee"
	}
	return formatInt(capacity) + " tickets * R" +
		formatFixed(repository.ListingFeePerTicket) + " = R" +
		formatFixed(total)
}

func formatInt(n int) string {
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n >= 1000 {
		s = "," + padThree(n%1000) + s
		n /= 1000
	}
	s = itoa(n) + s
	if neg {
		s = "-" + s
	}
	return s
}

func padThree(n int) string {
	s := itoa(n)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func formatFixed(v float64) string {
	whole := int(v)
	frac := int((v-float64(whole))*100 + 0.5)
	s := formatInt(whole) + "."
	if frac < 10 {
		s += "0"
	}
	s += itoa(frac)
	return s
}