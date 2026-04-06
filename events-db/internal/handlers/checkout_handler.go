package handlers

import (
	"errors"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// CheckoutBeginHandler creates ticket reservations for every ticket item in
// the cart. Merch items are not reserved — stock is checked at payment only.
func CheckoutBeginHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}

		var userID *string
		var sessionID *string
		if uid, ok := c.Get("user_id"); ok {
			s := uid.(string)
			userID = &s
		} else {
			sid := c.GetHeader("X-Session-ID")
			sessionID = &sid
		}

		reservations, err := repository.CreateReservations(pool, cart.ID, userID, sessionID)
		if err != nil {
			if errors.Is(err, repository.ErrInsufficientReservationCapacity) {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			if errors.Is(err, repository.ErrNoTicketItems) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create reservations"})
			return
		}

		eventIDs := make([]string, 0, len(reservations))
		seen := make(map[string]bool)
		for _, r := range reservations {
			if !seen[r.EventID] {
				eventIDs = append(eventIDs, r.EventID)
				seen[r.EventID] = true
			}
		}
		eventTitles, err := repository.GetEventTitlesByIDs(pool, eventIDs)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch event titles for checkout")
			return
		}

		shortest := 600
		summaries := make([]models.ReservationSummary, 0, len(reservations))
		for _, r := range reservations {
			secs := r.SecondsRemaining()
			if secs < shortest {
				shortest = secs
			}
			summaries = append(summaries, models.ReservationSummary{
				ReservationID: r.ID,
				EventID:       r.EventID,
				EventTitle:    eventTitles[r.EventID],
				Quantity:      r.Quantity,
				SecondsLeft:   secs,
			})
		}

		// Fix 1: use repository.SumCartTotal instead of inline SQL
		cartTotal, err := repository.SumCartTotal(pool, cart.ID)
		if err != nil {
			utils.InternalError(c, err, "failed to compute cart total")
			return
		}

		c.JSON(http.StatusOK, models.CheckoutBeginResponse{
			Reservations: summaries,
			SecondsLeft:  shortest,
			CartTotal:    cartTotal,
			Currency:     "ZAR",
		})
	}
}

// CheckoutHeartbeatHandler resets the 10-minute window on all active
// reservations for this cart. Call every 30 seconds from the frontend.
func CheckoutHeartbeatHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}

		secondsLeft, err := repository.RefreshReservations(pool, cart.ID)
		if err != nil {
			c.JSON(http.StatusGone, models.HeartbeatResponse{
				SecondsLeft: 0, Status: "expired",
			})
			return
		}

		c.JSON(http.StatusOK, models.HeartbeatResponse{
			SecondsLeft: secondsLeft, Status: "ok",
		})
	}
}

// CheckoutAbandonHandler releases all reservations for this cart immediately.
// Called via navigator.sendBeacon() on page unload — must return quickly.
func CheckoutAbandonHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusOK, gin.H{"released": false})
			return
		}
		// Fix 4: log instead of silently dropping the error
		if err := repository.AbandonReservations(pool, cart.ID); err != nil {
			log.Warn().Err(err).Str("cart_id", cart.ID).Msg("checkout: failed to abandon reservations")
		}
		c.JSON(http.StatusOK, gin.H{"released": true})
	}
}

// CheckoutConfirmHandler converts all active ticket reservations into real pending
// booking rows, then creates a single Stitch payment intent for the full cart total.
func CheckoutConfirmHandler(pool *pgxpool.Pool, paymentSvc *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}

		var userID *string
		var sessionID *string
		if uid, ok := c.Get("user_id"); ok {
			s := uid.(string)
			userID = &s
		} else {
			sid := c.GetHeader("X-Session-ID")
			if sid != "" {
				sessionID = &sid
			}
		}

		reservations, err := repository.GetReservationsByCart(pool, cart.ID)
		if err != nil || len(reservations) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no active reservations — please restart checkout"})
			return
		}

		bookingIDs, err := repository.ConvertReservationsToBookings(pool, cart.ID, userID, sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert reservations to bookings"})
			return
		}

		primaryBooking, err := repository.GetBookingByID(pool, bookingIDs[0])
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch primary booking"})
			return
		}

		// Fix 1: use repository.SumCartTotal instead of inline SQL
		cartTotal, err := repository.SumCartTotal(pool, cart.ID)
		if err != nil || cartTotal == 0 {
			cartTotal, err = repository.SumBookingTotalsByIDs(pool, bookingIDs)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute cart total"})
				return
			}
		}

		currency := primaryBooking.Currency
		primaryBooking.TotalAmount = cartTotal
		primaryBooking.Currency = currency

		clientToken, paymentRequestID, err := paymentSvc.CreateCartPaymentIntent(
			primaryBooking, cart.ID, cartTotal,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate payment"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"client_token":       clientToken,
			"payment_request_id": paymentRequestID,
			"total_amount":       cartTotal,
			"currency":           currency,
			"booking_ids":        bookingIDs,
			"primary_booking_id": bookingIDs[0],
		})
	}
}