package handlers
 
import (
  "errors"
  "events-db/internal/models"
  "events-db/internal/repository"
  "events-db/internal/services"
  "net/http"
 
  "github.com/gin-gonic/gin"
  "github.com/jackc/pgx/v5/pgxpool"
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
 
    // Build response with event titles
    shortest := 600
    summaries := make([]models.ReservationSummary, 0, len(reservations))
    for _, r := range reservations {
      secs := r.SecondsRemaining()
      if secs < shortest {
        shortest = secs
      }
      eventTitle := ""
      if event, eErr := repository.GetEventByID(pool, r.EventID, nil); eErr == nil {
        eventTitle = event.Title
      }
      summaries = append(summaries, models.ReservationSummary{
        ReservationID: r.ID,
        EventID:       r.EventID,
        EventTitle:    eventTitle,
        Quantity:      r.Quantity,
        SecondsLeft:   secs,
      })
    }
    // Compute cart total
    full, _ := repository.GetCartWithItems(pool, cart.ID)
    cartTotal := 0.0
    if full != nil {
      for _, item := range full.Items {
        cartTotal += item.UnitPriceSnapshot * float64(item.Quantity)
      }
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
    repository.AbandonReservations(pool, cart.ID)
    c.JSON(http.StatusOK, gin.H{"released": true})
  }
}
 
// CheckoutConfirmHandler converts all active ticket reservations into real pending
// booking rows, then creates a single Stitch payment intent for the full cart total.
//
// Flow:
//  1. Verify active reservations exist.
//  2. Call ConvertReservationsToBookings — each reservation becomes a real booking
//     row (status=pending), capacity moves from reserved_count → tickets_sold.
//  3. Create one Stitch payment intent for the combined total, linked to the
//     primary (first) booking AND to the cart_id so the webhook can confirm
//     every booking when payment completes.
//
// This replaces the old per-booking CreateStitchPayment flow for cart checkouts.
func CheckoutConfirmHandler(pool *pgxpool.Pool, paymentSvc *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}

		// Resolve user/session for booking creation
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

		// Verify at least one active reservation exists before touching bookings
		reservations, err := repository.GetReservationsByCart(pool, cart.ID)
		if err != nil || len(reservations) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no active reservations — please restart checkout"})
			return
		}

		// Convert every reservation into a real pending booking row.
		// This atomically: creates booking rows, moves reserved_count → tickets_sold,
		// and deletes the reservation rows.
		bookingIDs, err := repository.ConvertReservationsToBookings(pool, cart.ID, userID, sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert reservations to bookings"})
			return
		}

		// Fetch the primary booking to pass to the payment service
		primaryBooking, err := repository.GetBookingByID(pool, bookingIDs[0])
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch primary booking"})
			return
		}

		// Compute the full cart total across all items (tickets + merch)
		fullCart, _ := repository.GetCartWithItems(pool, cart.ID)
		cartTotal := 0.0
		currency := primaryBooking.Currency
		if fullCart != nil {
			for _, item := range fullCart.Items {
				cartTotal += item.UnitPriceSnapshot * float64(item.Quantity)
			}
		} else {
			// Fallback: sum from the bookings we just created
			for _, id := range bookingIDs {
				if b, bErr := repository.GetBookingByID(pool, id); bErr == nil {
					cartTotal += b.TotalAmount
				}
			}
		}

		// Override the primary booking's TotalAmount with the full cart total so
		// Stitch receives the correct combined amount.
		primaryBooking.TotalAmount = cartTotal
		primaryBooking.Currency = currency

		// Create one Stitch payment intent covering the entire cart.
		// The payment record is linked to the primary booking AND to cart_id so
		// the webhook can confirm ALL bookings when payment completes.
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
			"booking_ids":        bookingIDs, // all bookings covered by this payment
			"primary_booking_id": bookingIDs[0],
		})
	}
}