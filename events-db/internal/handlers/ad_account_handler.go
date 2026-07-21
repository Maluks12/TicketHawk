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
// ad_accounts.organizer_id references organizers.id — a separate UUID generated
// when the organizer profile was created — so we must look it up before any
// ad-account DB operation.
//
// Returns (organizerID, true) on success; writes a JSON error and returns
// ("", false) on failure so callers can return immediately.
func resolveOrganizerID(c *gin.Context, pool *pgxpool.Pool) (string, bool) {
	userID := c.GetString("user_id")
	org, err := repository.GetOrganizerByUserID(pool, userID)
	if err != nil {
		// In the KYC-gated flow this is reachable by plain attendees who hit
		// /v1/organiser/kyc before applying — point them at the right step.
		c.JSON(http.StatusNotFound, gin.H{"error": "No organiser application found — register as an organiser first"})
		return "", false
	}
	return org.ID, true
}

// CreateAdAccountHandler creates a new ad account for the organizer.
//
//	POST /organiser/ad-accounts
//	Body: { "name": "Main Account" }
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
			"message": "Ad account created. Top up your ad wallet to fund campaigns.",
			"next":    "POST /organiser/ad-wallet/topup",
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