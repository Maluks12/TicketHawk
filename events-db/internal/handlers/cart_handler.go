package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cartOwner resolves the cart for the current request.
// For authenticated users it uses user_id; for guests it uses the
// X-Session-ID header (a UUID the frontend generates and persists in localStorage).
func cartOwner(c *gin.Context, pool *pgxpool.Pool) (*models.Cart, error) {
	userIDVal, authed := c.Get("user_id")
	if authed {
		uid := userIDVal.(string)
		return repository.GetOrCreateCart(pool, &uid, nil)
	}
	sessionID := c.GetHeader("X-Session-ID")
	if sessionID == "" {
		return nil, nil
	}
	return repository.GetOrCreateCart(pool, nil, &sessionID)
}

func GetCartHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required for guest cart"})
			return
		}
		full, err := repository.GetCartWithItems(pool, cart.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load cart"})
			return
		}
		summary := buildCartSummary(full)
		c.JSON(http.StatusOK, summary)
	}
}

func AddCartItemHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required for guest cart"})
			return
		}

		var dto models.AddCartItemDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// ---------------------------------------------------------------
		// Resolve unit price
		// ---------------------------------------------------------------
		var price float64

		if dto.ItemType == models.CartItemTypeMerch {
			// Merch: price comes from the variant
			if dto.MerchandiseVariantID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "merchandise_variant_id required for merch items"})
				return
			}
			variant, basePrice, vErr := repository.GetVariantByID(pool, *dto.MerchandiseVariantID)
			if vErr != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "variant not found"})
				return
			}
			price = variant.EffectivePrice(basePrice)

		} else if dto.TicketTypeID != nil {
			// Ticket with a specific tier chosen: validate the type belongs to
			// the requested event and is currently on sale, then use its price.
			tt, ttErr := repository.GetTicketTypeByID(pool, *dto.TicketTypeID)
			if ttErr != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "ticket type not found"})
				return
			}
			if tt.EventID != dto.EventID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "ticket type does not belong to this event"})
				return
			}
			if !tt.IsOnSale() {
				c.JSON(http.StatusBadRequest, gin.H{"error": "this ticket type is not currently on sale"})
				return
			}
			tt.ComputeAvailable()
			if tt.IsSoldOut {
				c.JSON(http.StatusConflict, gin.H{"error": "this ticket type is sold out"})
				return
			}
			price = tt.Price

		} else {
			// Flat-priced ticket: fall back to events.price
			event, eErr := repository.GetEventByID(pool, dto.EventID, nil)
			if eErr != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
				return
			}
			// If the event has ticket types but none was specified, reject the
			// request so the frontend always selects a tier explicitly.
			if len(event.TicketTypes) > 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "this event has multiple ticket types — please include ticket_type_id",
				})
				return
			}
			price = event.Price
		}

		item, err := repository.AddCartItem(pool, cart.ID, &dto, price)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, item)
	}
}

func UpdateCartItemHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}
		itemID := c.Param("id")
		var dto models.UpdateCartItemDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := repository.UpdateCartItemQuantity(pool, cart.ID, itemID, dto.Quantity); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "quantity updated"})
	}
}

func DeleteCartItemHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}
		itemID := c.Param("id")
		if err := repository.DeleteCartItem(pool, cart.ID, itemID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "item removed"})
	}
}

func ClearCartHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cart, err := cartOwner(c, pool)
		if err != nil || cart == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
			return
		}
		if err := repository.ClearCart(pool, cart.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cart"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "cart cleared"})
	}
}

func MergeCartHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		var dto models.MergeCartDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := repository.MergeGuestCart(pool, userID.(string), dto.SessionID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "cart merge failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "cart merged"})
	}
}

// buildCartSummary groups cart items by event and computes totals.
func buildCartSummary(cart *models.Cart) models.CartSummary {
	groups := map[string]*models.EventGroup{}
	var total float64

	for _, item := range cart.Items {
		itemTotal := item.UnitPriceSnapshot * float64(item.Quantity)
		total += itemTotal

		eg, ok := groups[item.EventID]
		if !ok {
			title := ""
			if item.EventTitle != nil {
				title = *item.EventTitle
			}
			groups[item.EventID] = &models.EventGroup{
				EventID: item.EventID, EventTitle: title,
			}
			eg = groups[item.EventID]
		}
		eg.Items = append(eg.Items, item)
		eg.Subtotal += itemTotal
	}

	summary := models.CartSummary{
		TotalItems:  len(cart.Items),
		TotalAmount: total,
		Currency:    "ZAR",
	}
	for _, eg := range groups {
		summary.ItemsByEvent = append(summary.ItemsByEvent, *eg)
	}
	return summary
}