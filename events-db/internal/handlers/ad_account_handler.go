package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateAdAccountHandler creates a new ad account for the organizer.
//
//	POST /organiser/ad-accounts
//	Body: { "name": "Thetsongaboys Main Account" }
func CreateAdAccountHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")

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
		orgID := c.GetString("user_id")

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
		orgID     := c.GetString("user_id")
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
		orgID     := c.GetString("user_id")
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
				c.JSON(http.StatusForbidden, gin.H{"error": "Not your ad account"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update ad account"})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{"account": account, "message": "Ad account updated"})
	}
}

// RegisterAdAccountPaymentMethodHandler stores a Stitch mandate against a
// specific ad account. The organizer completes the Stitch mandate OAuth flow
// in the frontend, receives a mandate ID, then calls this endpoint once.
//
//	POST /organiser/ad-accounts/:id/payment-method
//	Body: { "stitch_mandate_id": "man_abc123", "email": "billing@example.com" }
func RegisterAdAccountPaymentMethodHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID     := c.GetString("user_id")
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
			"billing":  "Campaigns under this account will be billed automatically each morning for the previous day's actual spend.",
		})
	}
}

// GetAdAccountPaymentMethodHandler returns the payment method status for an account.
//
//	GET /organiser/ad-accounts/:id/payment-method
func GetAdAccountPaymentMethodHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID     := c.GetString("user_id")
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