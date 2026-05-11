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
			if !errors.Is(err, repository.ErrNoTicketItems) {
				// ErrNoTicketItems is not an error — it just means a merch-only
				// cart. Every other error is a real failure.
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create reservations"})
				return
			}
			// Merch-only cart: treat as zero reservations and fall through.
			created = nil
		}

		// Build reservation summaries (empty slice for merch-only carts).
		shortest := 600
		summaries := make([]models.ReservationSummary, 0, len(created))

		if len(created) > 0 {
			eventIDs := make([]string, 0, len(created))
			seen := make(map[string]bool)
			for _, r := range created {
				if !seen[r.EventID] {
					eventIDs = append(eventIDs, r.EventID)
					seen[r.EventID] = true
				}
			}
			eventTitles, titlesErr := events.GetEventTitlesByIDs(eventIDs)
			if titlesErr != nil {
				utils.InternalError(c, titlesErr, "failed to fetch event titles for checkout")
				return
			}
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
		} else {
			// No ticket items — merch-only cart, no countdown needed.
			shortest = 0
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
			Currency:     models.DefaultCurrency,
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

		// ── Ticket reservations → bookings ──────────────────────────────────────
		// For merch-only carts there are no reservations, which is fine.
		// For ticket (or mixed) carts, reservations must still be alive.
		existing, resErr := reservations.GetReservationsByCart(cart.ID)
		hasTicketReservations := resErr == nil && len(existing) > 0

		// Check whether the cart actually contains ticket items.
		allCartItems := cart.Items
		hasTickets := false
		for _, item := range allCartItems {
			if item.ItemType == models.CartItemTypeTicket {
				hasTickets = true
				break
			}
		}

		if hasTickets && !hasTicketReservations {
			// Ticket items are present but reservations have expired or were never created.
			c.JSON(http.StatusBadRequest, gin.H{"error": "reservations expired — please restart checkout"})
			return
		}

		cartTotal, cartTotalErr := carts.SumCartTotal(cart.ID)
		if cartTotalErr != nil || cartTotal == 0 {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute cart total"})
			return
		}

		var primaryBookingID string
		var bookingIDs []string
		currency := models.DefaultCurrency

		if hasTickets {
			// Convert reservations to real booking rows.
			var convErr error
			bookingIDs, convErr = reservations.ConvertReservationsToBookings(cart.ID, userID, sessionID)
			if convErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert reservations to bookings"})
				return
			}
			primaryBookingID = bookingIDs[0]

			primaryBooking, fetchErr := bookings.GetBookingByID(primaryBookingID)
			if fetchErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch primary booking"})
				return
			}
			currency = primaryBooking.Currency

			// Create the Stitch payment intent using the primary booking.
			primaryBooking.TotalAmount = cartTotal
			clientToken, paymentRequestID, payErr := paymentSvc.CreateCartPaymentIntent(primaryBooking, cart.ID, cartTotal)
			if payErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate payment"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"client_token":       clientToken,
				"payment_request_id": paymentRequestID,
				"total_amount":       cartTotal,
				"currency":           currency,
				"booking_ids":        bookingIDs,
				"primary_booking_id": primaryBookingID,
			})
			return
		}

		// ── Merch-only cart ───────────────────────────────────────────────────
		// No ticket booking exists yet. Create a synthetic merch-only booking row
		// so the payment record has something to anchor to.
		merchBooking, mbErr := bookings.CreateMerchOnlyBooking(cart.ID, userID, sessionID, cartTotal, currency)
		if mbErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create merch booking"})
			return
		}
		primaryBookingID = merchBooking.ID
		bookingIDs = []string{primaryBookingID}

		clientToken, paymentRequestID, payErr := paymentSvc.CreateCartPaymentIntent(merchBooking, cart.ID, cartTotal)
		if payErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate payment"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"client_token":       clientToken,
			"payment_request_id": paymentRequestID,
			"total_amount":       cartTotal,
			"currency":           currency,
			"booking_ids":        bookingIDs,
			"primary_booking_id": primaryBookingID,
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