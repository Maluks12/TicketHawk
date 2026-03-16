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
 
// CheckoutConfirmHandler creates a Stitch payment intent for the full cart total.
// This replaces the per-booking CreateStitchPayment flow.
func CheckoutConfirmHandler(pool *pgxpool.Pool, paymentSvc *services.PaymentService) gin.HandlerFunc {
  return func(c *gin.Context) {
    cart, err := cartOwner(c, pool)
    if err != nil || cart == nil {
      c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
      return
    }
 
    // Verify at least one active reservation exists
    reservations, err := repository.GetReservationsByCart(pool, cart.ID)
    if err != nil || len(reservations) == 0 {
      c.JSON(http.StatusBadRequest, gin.H{"error": "no active reservations — please restart checkout"})
      return
    }
 
    // Use first reservation's booking_number as payer reference for Stitch
    // (real multi-booking payment intent is a future enhancement)
    r := reservations[0]
    full, _ := repository.GetCartWithItems(pool, cart.ID)
    cartTotal := 0.0
    currency := "ZAR"
    if full != nil {
      for _, item := range full.Items {
        cartTotal += item.UnitPriceSnapshot * float64(item.Quantity)
      }
    }
 
    // Build a synthetic Booking for the payment service
    syntheticBooking := &models.Booking{
      ID:            r.CartID, // cart ID as the external reference
      BookingNumber: r.BookingNumber,
      TotalAmount:   cartTotal,
      Currency:      currency,
    }
 
    clientToken, paymentRequestID, err := paymentSvc.CreatePaymentIntent(syntheticBooking)
    if err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate payment"})
      return
    }
 
    c.JSON(http.StatusOK, gin.H{
      "client_token":       clientToken,
      "payment_request_id": paymentRequestID,
      "total_amount":       cartTotal,
      "currency":           currency,
    })
  }
}