package handlers

import (
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MinWalletTopup is the smallest allowed single top-up (ZAR).
const MinWalletTopup = 50.0

// GetWalletBalanceHandler returns the organiser's ad wallet balance and totals.
//
//	GET /organiser/ad-wallet
func GetWalletBalanceHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")

		wallet, err := repository.GetOrCreateAdWallet(pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch wallet"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"balance":      wallet.Balance,
			"total_topups": wallet.TotalTopups,
			"total_spent":  wallet.TotalSpent,
			"currency":     models.DefaultCurrency,
			"can_serve":    wallet.Balance > 0,
			"note":         "Campaigns serve only while the wallet balance is positive.",
		})
	}
}

// InitiateWalletTopupHandler starts a card (Paystack) top-up. The balance is
// credited only once the Paystack webhook confirms the payment.
//
//	POST /organiser/ad-wallet/topup
//	Body: { "amount": 500 }
func InitiateWalletTopupHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")

		var dto struct {
			Amount float64 `json:"amount" binding:"required"`
		}
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if dto.Amount < MinWalletTopup {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Minimum top-up is R50",
			})
			return
		}

		redirectURL, reference, err := paymentService.CreateAdTopupIntent(orgID, dto.Amount)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Payment service temporarily unavailable. Please try again shortly.",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"authorization_url": redirectURL,
			"reference":         reference,
			"amount":            dto.Amount,
			"currency":          models.DefaultCurrency,
			"message":           "Complete the card payment to credit your ad wallet.",
		})
	}
}

// GetWalletTopupHistoryHandler returns the organiser's past top-ups.
//
//	GET /organiser/ad-wallet/topups?limit=30
func GetWalletTopupHistoryHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("user_id")

		limit := 30
		if l := c.Query("limit"); l != "" {
			if n := parseLimit(l); n > 0 {
				limit = n
			}
		}

		topups, err := repository.GetAdWalletTopups(pool, orgID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch top-up history"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"topups": topups,
			"count":  len(topups),
		})
	}
}

func parseLimit(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
		if n > 90 {
			return 90
		}
	}
	return n
}