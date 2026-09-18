package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Refunds and manual adjustments ────────────────────────────────────────────
//
// Two paths with deliberately different authorisation:
//
//	Refund      — the TILL, because it is the cashier facing the attendee who
//	              handed back a drink. Requires a device token, and the refund
//	              may only reverse a sale that same vendor made.
//	Adjustment  — a MANAGER, because it moves money on somebody's word alone
//	              with no sale behind it. Requires a reason, which the database
//	              also enforces.

// LookupRefundableHandler shows a cashier what remains before they commit.
//
//	GET /v1/staff/events/:id/cashless/refundable?txn_id=...
//
// A confirm screen rather than a blind refund: "R20 of this R50 sale is still
// refundable" prevents most of the mistakes the checks would otherwise catch
// after somebody has already told the attendee yes.
func LookupRefundableHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		txnID := c.Query("txn_id")
		if txnID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "txn_id is required"})
			return
		}

		rc, remaining, err := repository.GetRefundableTransaction(ctx, pool, eventID, txnID)
		if errors.Is(err, repository.ErrTxnNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found for this event"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load transaction"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"kind":             rc.OriginalKind,
			"original_amount":  repository.FromCents(rc.OriginalCents),
			"already_refunded": repository.FromCents(rc.AlreadyRefundedCents),
			"refundable":       remaining,
			"refundable_by":    rc.VendorID,
		})
	}
}

// RefundHandler reverses part or all of a sale.
//
//	POST /v1/staff/events/:id/cashless/refund
//	{ "txn_id": "<uuid>", "device_token": "...", "original_txn_id": "<uuid>",
//	  "amount": 37.50, "reason": "wrong item served" }
//
// txn_id is CLIENT-generated. The stakes are higher than for a purchase: a
// duplicate refund pays an attendee twice out of the vendor's settlement, so a
// till that loses its response and retries must land on the same row.
func RefundHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}
		operatorID := c.MustGet("user_id").(string)

		var in struct {
			TxnID         string  `json:"txn_id" binding:"required,uuid"`
			DeviceToken   string  `json:"device_token" binding:"required"`
			OriginalTxnID string  `json:"original_txn_id" binding:"required,uuid"`
			Amount        float64 `json:"amount" binding:"required,gt=0"`
			Reason        string  `json:"reason" binding:"required,min=3,max=200"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// The vendor comes from the DEVICE TOKEN, never from the request body.
		// A till claiming to be another vendor would be able to refund that
		// vendor's sales out of their settlement.
		deviceID, vendorID, err := repository.ResolvePOSDevice(ctx, pool, eventID, in.DeviceToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unrecognised or suspended till"})
			return
		}

		amountCents, err := repository.ToCents(in.Amount)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount"})
			return
		}

		txn, err := repository.RecordRefund(ctx, pool, eventID, in.TxnID,
			in.OriginalTxnID, vendorID, deviceID, amountCents, in.Reason, operatorID)
		if err != nil {
			var refusal *repository.RefundRefusal
			if errors.As(err, &refusal) {
				status := http.StatusConflict
				if refusal.Reason == repository.RefundNoOriginal {
					status = http.StatusNotFound
				}
				c.JSON(status, gin.H{
					"error":  refundMessage(refusal.Reason),
					"reason": refusal.Reason,
					// Returned so the till can offer the partial rather than
					// making the cashier guess and try again.
					"refundable":      refusal.RemainingRefund,
					"original_amount": refusal.OriginalAmount,
				})
				return
			}
			log.Error().Err(err).Str("event_id", eventID).
				Str("original", in.OriginalTxnID).Msg("cashless: refund failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record refund"})
			return
		}

		w, _ := repository.GetWallet(ctx, pool, txn.WalletID)
		log.Info().Str("event_id", eventID).Str("vendor_id", vendorID).
			Str("original", in.OriginalTxnID).Float64("amount", in.Amount).
			Msg("cashless: refund issued")

		c.JSON(http.StatusCreated, gin.H{
			"transaction": txn,
			"balance":     w.Balance,
		})
	}
}

// refundMessage turns a machine reason into something a cashier with somebody
// waiting can act on. Each maps to a different next step.
func refundMessage(reason string) string {
	switch reason {
	case repository.RefundExceedsSale:
		return "That is more than is left on this sale — check the refundable amount"
	case repository.RefundNotAPurchase:
		return "Only a sale can be refunded, and this is not one"
	case repository.RefundWrongVendor:
		return "This sale was made at a different stall — they have to refund it"
	case repository.RefundNoOriginal:
		return "That sale could not be found"
	case repository.RefundWalletClosed:
		return "This wallet is closed — send them to the help desk"
	case repository.RefundNoReason:
		return "A reason is required"
	default:
		return "Refund refused"
	}
}

// AdjustmentHandler applies a manual correction.
//
//	POST /v1/staff/events/:id/cashless/adjustment
//	{ "wallet_id": "<uuid>", "amount": -25.00, "reason": "charged twice at Bar 1" }
//
// MANAGER-only. This is the one path that moves money in either direction on
// somebody's word alone, with no sale behind it, so it does not go through the
// till. The amount is SIGNED: negative debits, positive credits.
func AdjustmentHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}
		managerID := c.MustGet("user_id").(string)

		var in struct {
			WalletID string  `json:"wallet_id" binding:"required,uuid"`
			Amount   float64 `json:"amount" binding:"required"`
			Reason   string  `json:"reason" binding:"required,min=5,max=200"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		signedCents, err := repository.ToCents(in.Amount)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount"})
			return
		}

		txn, err := repository.RecordAdjustment(ctx, pool, eventID, in.WalletID,
			signedCents, in.Reason, managerID)
		if err != nil {
			var refusal *repository.SpendRefusal
			if errors.As(err, &refusal) {
				c.JSON(http.StatusConflict, gin.H{
					"error":   adjustmentMessage(refusal.Reason),
					"reason":  refusal.Reason,
					"balance": refusal.Balance,
				})
				return
			}
			if errors.Is(err, repository.ErrWalletNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found for this event"})
				return
			}
			log.Error().Err(err).Str("wallet_id", in.WalletID).
				Msg("cashless: adjustment failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record adjustment"})
			return
		}

		// Warn rather than info. A manual adjustment is a human moving somebody
		// else's money, and it should be findable in a log without effort.
		log.Warn().Str("event_id", eventID).Str("wallet_id", in.WalletID).
			Str("by", managerID).Float64("amount", in.Amount).
			Str("reason", in.Reason).Msg("cashless: MANUAL ADJUSTMENT")

		w, _ := repository.GetWallet(ctx, pool, in.WalletID)
		c.JSON(http.StatusCreated, gin.H{"transaction": txn, "balance": w.Balance})
	}
}

func adjustmentMessage(reason string) string {
	switch reason {
	case repository.AdjustmentWouldGoNegative:
		return "That would take the wallet below zero — reduce the amount"
	case repository.AdjustmentTooLarge:
		return "That amount looks wrong — check the decimal point"
	case repository.AdjustmentNoReason:
		return "A reason is required"
	default:
		return "Adjustment refused"
	}
}