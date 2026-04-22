package handlers

import (
	"errors"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// FIX 3: All four checkout handlers previously accepted *pgxpool.Pool directly
// and called repository package functions inline, which made them impossible to
// unit-test without a live database. They now accept the same narrow repository
// interfaces used by the booking handlers.
//
// Wire-up in main.go (replace the old pool-based calls):
//
//   cartQuerier     := repository.NewPgxCartQuerier(pool)
//   reserveQuerier  := repository.NewPgxReservationQuerier(pool)
//   eventQuerier    := repository.NewPgxEventQuerier(pool)
//   bookingQuerier  := repository.NewPgxBookingQuerier(pool)
//
//   checkout.POST("/begin",     middleware.OptionalAuth(cfg), handlers.CheckoutBeginHandler(cartQuerier, reserveQuerier, eventQuerier))
//   checkout.POST("/heartbeat", middleware.OptionalAuth(cfg), handlers.CheckoutHeartbeatHandler(cartQuerier, reserveQuerier))
//   checkout.POST("/abandon",   middleware.OptionalAuth(cfg), handlers.CheckoutAbandonHandler(cartQuerier, reserveQuerier))
//   checkout.POST("/confirm",   middleware.OptionalAuth(cfg), handlers.CheckoutConfirmHandler(cartQuerier, reserveQuerier, bookingQuerier, paymentService))

// CheckoutBeginHandler creates ticket reservations for every ticket item in
// the cart. Merch items are not reserved — stock is checked at payment only.
func CheckoutBeginHandler(
	carts repository.CartQuerier,
	reservations repository.ReservationQuerier,
	events repository.EventQuerier,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwnerFromQuerier(c, carts)
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

		created, err := reservations.CreateReservations(cart.ID, userID, sessionID)
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

		eventIDs := make([]string, 0, len(created))
		seen := make(map[string]bool)
		for _, r := range created {
			if !seen[r.EventID] {
				eventIDs = append(eventIDs, r.EventID)
				seen[r.EventID] = true
			}
		}
		eventTitles, err := events.GetEventTitlesByIDs(eventIDs)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch event titles for checkout")
			return
		}

		shortest := 600
		summaries := make([]models.ReservationSummary, 0, len(created))
		for _, r := range created {
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

		cartTotal, err := carts.SumCartTotal(cart.ID)
		if err != nil {
			utils.InternalError(c, err, "failed to compute cart total")
			return
		}

		c.JSON(http.StatusOK, models.CheckoutBeginResponse{
			Reservations: summaries,
			SecondsLeft:  shortest,
			CartTotal:    cartTotal,
			Currency:     models.DefaultCurrency, // FIX 6: use constant
		})
	}
}

// CheckoutHeartbeatHandler resets the 10-minute window on all active
// reservations for this cart. Call every 30 seconds from the frontend.
func CheckoutHeartbeatHandler(
	carts repository.CartQuerier,
	reservations repository.ReservationQuerier,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwnerFromQuerier(c, carts)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}

		secondsLeft, err := reservations.RefreshReservations(cart.ID)
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
func CheckoutAbandonHandler(
	carts repository.CartQuerier,
	reservations repository.ReservationQuerier,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwnerFromQuerier(c, carts)
		if err != nil || cart == nil {
			c.JSON(http.StatusOK, gin.H{"released": false})
			return
		}
		if err := reservations.AbandonReservations(cart.ID); err != nil {
			log.Warn().Err(err).Str("cart_id", cart.ID).Msg("checkout: failed to abandon reservations")
		}
		c.JSON(http.StatusOK, gin.H{"released": true})
	}
}

// CheckoutConfirmHandler converts all active ticket reservations into real
// pending booking rows, then creates a single Stitch payment intent for the
// full cart total.
func CheckoutConfirmHandler(
	carts repository.CartQuerier,
	reservations repository.ReservationQuerier,
	bookings repository.BookingQuerier,
	paymentSvc *services.PaymentService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwnerFromQuerier(c, carts)
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

		existing, err := reservations.GetReservationsByCart(cart.ID)
		if err != nil || len(existing) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no active reservations — please restart checkout"})
			return
		}

		bookingIDs, err := reservations.ConvertReservationsToBookings(cart.ID, userID, sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert reservations to bookings"})
			return
		}

		primaryBooking, err := bookings.GetBookingByID(bookingIDs[0])
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch primary booking"})
			return
		}

		cartTotal, err := carts.SumCartTotal(cart.ID)
		if err != nil || cartTotal == 0 {
			cartTotal, err = bookings.SumBookingTotalsByIDs(bookingIDs)
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

// cartOwnerFromQuerier resolves the cart for the current request using the
// CartQuerier interface instead of a raw pool, keeping checkout testable.
func cartOwnerFromQuerier(c *gin.Context, carts repository.CartQuerier) (*models.Cart, error) {
	if userIDVal, authed := c.Get("user_id"); authed {
		uid := userIDVal.(string)
		return carts.GetOrCreateCart(&uid, nil)
	}
	sessionID := c.GetHeader("X-Session-ID")
	if sessionID == "" {
		return nil, nil
	}
	return carts.GetOrCreateCart(nil, &sessionID)
}