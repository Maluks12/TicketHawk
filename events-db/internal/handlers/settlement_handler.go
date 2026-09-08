package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Vendor settlement & float reconciliation ──────────────────────────────────
//
// ORGANISER-owned. Deciding what a vendor is paid, and reading how much of the
// float is actually theirs, are commercial decisions — a scanner supervisor
// should not be able to make either.

// CalculateSettlementsHandler works out what every vendor is owed for a period.
//
//	POST /v1/organiser/events/:id/cashless/settlements/calculate
//	{ "period_start": "2026-09-05T18:00:00Z", "period_end": "2026-09-06T04:00:00Z" }
//
// Produces DRAFTS. Nothing is approved and no money moves — an organiser reads
// the numbers, checks the flagged column, and approves separately.
//
// Re-running the same period updates the drafts rather than creating a second
// set, and refuses to touch anything already approved or paid. Recalculating a
// payout after the money left would silently rewrite what both sides agreed.
func CalculateSettlementsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		eventID := requireEventOwner(c, ctx, pool)
		if eventID == "" {
			return
		}
		userID := c.MustGet("user_id").(string)

		var in struct {
			PeriodStart time.Time `json:"period_start" binding:"required"`
			PeriodEnd   time.Time `json:"period_end" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !in.PeriodEnd.After(in.PeriodStart) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "period_end must be after period_start"})
			return
		}

		totals, err := repository.VendorTotalsForPeriod(ctx, pool, eventID, in.PeriodStart, in.PeriodEnd)
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("settlement: totals failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to compute totals"})
			return
		}

		type outcome struct {
			Vendor     string   `json:"vendor"`
			Code       string   `json:"code"`
			Gross      float64  `json:"gross"`
			Commission float64  `json:"commission"`
			Net        float64  `json:"net"`
			TxnCount   int      `json:"txn_count"`
			Flagged    int      `json:"flagged_count"`
			Error      string   `json:"error,omitempty"`
			ID         string   `json:"settlement_id,omitempty"`
		}

		results := make([]outcome, 0, len(totals))
		var totalNet, totalCommission int64
		skipped := 0

		for _, t := range totals {
			o := outcome{Vendor: t.VendorName, Code: t.VendorCode,
				TxnCount: t.TxnCount, Flagged: t.FlaggedCount}

			calc, err := repository.CalculateSettlement(repository.SettlementInput{
				GrossCents: t.GrossCents, RefundCents: t.RefundCents,
				CommissionRate: t.CommissionRate,
			})
			if err != nil {
				// A bad commission rate on one vendor must not abort the run for
				// everyone else — the organiser fixes that vendor and re-runs.
				o.Error = err.Error()
				results = append(results, o)
				continue
			}

			id, err := repository.UpsertSettlement(ctx, pool, eventID, t.VendorID,
				in.PeriodStart, in.PeriodEnd, t, *calc, userID)
			if err != nil {
				// Most likely an existing non-draft settlement for this period,
				// which the upsert refuses to overwrite. Reported rather than
				// treated as a failure: that is the guard working.
				o.Error = "already approved or paid for this period — not recalculated"
				results = append(results, o)
				skipped++
				continue
			}

			o.ID = id
			o.Gross = repository.FromCents(calc.GrossCents)
			o.Commission = repository.FromCents(calc.CommissionCents)
			o.Net = repository.FromCents(calc.NetCents)
			totalNet += calc.NetCents
			totalCommission += calc.CommissionCents
			results = append(results, o)
		}

		resp := gin.H{
			"settlements": results,
			"totals": gin.H{
				"net_payable":      repository.FromCents(totalNet),
				"commission_taken": repository.FromCents(totalCommission),
				"vendors":          len(results),
				"skipped":          skipped,
			},
			"status": "draft — nothing has been approved and no money has moved",
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListSettlementsHandler shows every settlement for an event.
//
//	GET /v1/organiser/events/:id/cashless/settlements
func ListSettlementsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		eventID := requireEventOwner(c, ctx, pool)
		if eventID == "" {
			return
		}

		rows, err := repository.ListSettlements(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load settlements"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"settlements": rows})
	}
}

// UpdateSettlementStatusHandler approves, pays, disputes, or cancels.
//
//	POST /v1/organiser/events/:id/cashless/settlements/status
//	{ "settlement_id": "...", "status": "approved" }
//	{ "settlement_id": "...", "status": "paid", "reference": "EFT-20260907-001" }
//
// Transitions are guarded in SQL: approve only from draft, pay only from
// approved, and nothing at all once paid. Marking something paid that was never
// approved would mean money leaving with nobody signing it off.
func UpdateSettlementStatusHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		eventID := requireEventOwner(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			SettlementID string `json:"settlement_id" binding:"required,uuid"`
			Status       string `json:"status" binding:"required"`
			Reference    string `json:"reference" binding:"max=64"`
			Note         string `json:"note" binding:"max=500"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Scoped to the caller's event. Without this, an organiser could move
		// somebody else's settlement by guessing an id.
		var owns bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM vendor_settlements WHERE id = $1 AND event_id = $2)`,
			in.SettlementID, eventID).Scan(&owns); err != nil || !owns {
			c.JSON(http.StatusNotFound, gin.H{"error": "Settlement not found for this event"})
			return
		}

		err := repository.SetSettlementStatus(ctx, pool, in.SettlementID,
			in.Status, in.Reference, in.Note)
		switch {
		case errors.Is(err, repository.ErrInvalidSettlementStatus):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Unknown status",
				"allowed": []string{"approved", "paid", "disputed", "cancelled"},
			})
		case errors.Is(err, repository.ErrSettlementTransition):
			c.JSON(http.StatusConflict, gin.H{
				"error": "That transition is not allowed. Approve before paying, " +
					"and a paid settlement cannot be changed.",
			})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settlement"})
		default:
			if in.Status == "paid" {
				log.Info().Str("event_id", eventID).Str("settlement_id", in.SettlementID).
					Str("reference", in.Reference).Msg("settlement marked paid")
			}
			c.JSON(http.StatusOK, gin.H{"message": "Settlement updated", "status": in.Status})
		}
	}
}

// FloatReconciliationHandler reports where an event's cashless money sits.
//
//	GET  /v1/organiser/events/:id/cashless/float          (read only)
//	POST /v1/organiser/events/:id/cashless/float          (read and record)
//
// The number that matters is outstanding_liability: money attendees topped up
// and have not spent. It is owed back under the §6 unspent-balance policy and
// is NEVER revenue. An organiser treating the closing float as income is
// spending attendees' money, and that is also what the money-transmitter
// question in §10 turns on.
func FloatReconciliationHandler(pool *pgxpool.Pool, record bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()

		eventID := requireEventOwner(c, ctx, pool)
		if eventID == "" {
			return
		}

		in, err := repository.EventFloatTotals(ctx, pool, eventID)
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("float: totals failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to compute float"})
			return
		}
		res := repository.CalculateFloat(in)

		shortfallCount, shortfallCents, err := repository.EventShortfallTotals(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load shortfalls"})
			return
		}

		body := gin.H{
			"money_in": gin.H{
				"topups": repository.FromCents(in.TopupsCents),
			},
			"money_out": gin.H{
				"purchases":         repository.FromCents(in.PurchasesCents),
				"refunds_to_wallets": repository.FromCents(in.RefundsCents),
				"payouts_to_attendees": repository.FromCents(in.PayoutsCents),
				"settled_to_vendors":   repository.FromCents(in.SettledCents),
			},
			"obligations": gin.H{
				"owed_to_attendees": repository.FromCents(res.OutstandingLiabilityCents),
				"owed_to_vendors":   repository.FromCents(res.VendorsOwedCents),
				// Deliberately the SUM, never the difference. Two debts to
				// different people that happen to be equal are not square.
				"total_held": repository.FromCents(res.HeldCents),
			},
			"shortfalls": gin.H{
				"count":  shortfallCount,
				"amount": repository.FromCents(shortfallCents),
			},
			"balanced": res.Balanced,
			"note": "owed_to_attendees is a LIABILITY under the unspent-balance policy, " +
				"not revenue. total_held is what must be in the bank to meet both obligations.",
		}

		if !res.Balanced {
			// Vendors have been paid for sales the ledger has no record of.
			// Integer cents throughout and two independently written tables, so
			// this is never rounding — it is a real inconsistency.
			body["warning"] = "Vendors have been settled R" +
				strconv.FormatFloat(repository.FromCents(res.OverSettledCents), 'f', 2, 64) +
				" more than was ever spent with them. Investigate before paying anyone else."
			body["over_settled"] = repository.FromCents(res.OverSettledCents)
			log.Error().Str("event_id", eventID).
				Int64("over_settled_cents", res.OverSettledCents).
				Msg("float: vendor settlements exceed recorded vendor spend")
		}

		if record {
			id, err := repository.SaveFloatReconciliation(ctx, pool, eventID, in, res,
				shortfallCount, shortfallCents, res.VendorsOwedCents,
				c.MustGet("user_id").(string))
			if err != nil {
				log.Error().Err(err).Msg("float: could not record reconciliation")
			} else {
				body["reconciliation_id"] = id
			}
		}

		c.JSON(http.StatusOK, body)
	}
}