package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// resolveOrganizerID translates the JWT user_id into the organizer's own primary key.
//
// The JWT stores the user's UUID. ad_accounts.organizer_id references
// organizers.id — a separate UUID generated when the organizer profile was
// created — so we must look it up before any ad-account DB operation.
//
// Returns (organizerID, true) on success; writes a JSON error and returns
// ("", false) on failure so callers can return immediately.
func resolveOrganizerID(c *gin.Context, pool *pgxpool.Pool) (string, bool) {
	userID := c.GetString("user_id")
	org, err := repository.GetOrganizerByUserID(pool, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Organizer profile not found"})
		return "", false
	}
	return org.ID, true
}

// CreateAdAccountHandler creates a new ad account for the organizer.
//
//	POST /organiser/ad-accounts
//	Body: { "name": "Thetsongaboys Main Account" }
func CreateAdAccountHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}

		var dto models.CreateAdAccountDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		account, err := repository.CreateAdAccount(pool, orgID, &dto)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create ad account"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"account": account,
			"message": "Ad account created. Add a payment method before running campaigns.",
			"next":    "POST /organiser/ad-accounts/" + account.ID + "/payment-method",
		})
	}
}

// GetAdAccountsHandler returns all ad accounts for the organizer.
//
//	GET /organiser/ad-accounts
func GetAdAccountsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}

		accounts, err := repository.GetAdAccounts(pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch ad accounts"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"accounts": accounts,
			"count":    len(accounts),
		})
	}
}

// GetAdAccountHandler returns a single ad account.
//
//	GET /organiser/ad-accounts/:id
func GetAdAccountHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}
		accountID := c.Param("id")

		account, err := repository.GetAdAccount(pool, accountID, orgID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ad account not found"})
			return
		}

		c.JSON(http.StatusOK, account)
	}
}

// UpdateAdAccountHandler updates the name or status of an ad account.
//
//	PATCH /organiser/ad-accounts/:id
//	Body: { "name": "New Name" } or { "status": "disabled" }
func UpdateAdAccountHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}
		accountID := c.Param("id")

		var dto models.UpdateAdAccountDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		account, err := repository.UpdateAdAccount(pool, accountID, orgID, &dto)
		if err != nil {
			switch err {
			case repository.ErrAdAccountNotFound:
				c.JSON(http.StatusNotFound, gin.H{"error": "Ad account not found"})
			case repository.ErrAdAccountUnauthorized:
				c.JSON(http.StatusNotFound, gin.H{"error": "Ad account not found"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update ad account"})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{"account": account, "message": "Ad account updated"})
	}
}

// RegisterAdAccountPaymentMethodHandler stores card payment details against a
// specific ad account. Only safe display fields are stored (last4, brand, expiry) —
// raw card numbers and CVV are never sent here.
//
//	POST /organiser/ad-accounts/:id/payment-method
//	Body: {
//	  "card_name":   "John Doe",
//	  "card_last4":  "4242",
//	  "card_brand":  "Visa",
//	  "card_expiry": "08/27",
//	  "email":       "billing@example.com"
//	}
func RegisterAdAccountPaymentMethodHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}
		accountID := c.Param("id")

		var dto models.RegisterAdAccountPaymentMethodDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.RegisterAdAccountPaymentMethod(pool, accountID, orgID, &dto); err != nil {
			if err == repository.ErrAdAccountNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Ad account not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register payment method"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Payment method registered",
			"status":   "active",
			"has_payment_method": true,
			"billing":  "Campaigns under this account will be billed automatically each morning for the previous day's actual spend.",
		})
	}
}

// GetAdAccountPaymentMethodHandler returns the payment method status for an account.
//
//	GET /organiser/ad-accounts/:id/payment-method
func GetAdAccountPaymentMethodHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}
		accountID := c.Param("id")

		status, email, err := repository.GetAdAccountPaymentMethodStatus(pool, accountID, orgID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ad account not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":            status,
			"has_payment_method": status == "active",
			"ad_payment_email":  email,
		})
	}
}