package handlers

import (
	"context"
	"net/http"
	"time"

	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Offline sync & mandate capture (Phase 4d) ─────────────────────────────────
//
// Mirrors IngestScansHandler: the till posts its queue, the server decides
// every verdict, and per-item outcomes come back so the device knows exactly
// what to drop from its queue and what to keep.

// SyncOfflineTxnsHandler ingests a till's queued offline sales.
//
//	POST /v1/staff/events/:id/pos/offline/sync
//	{ "device_token": "...", "transactions": [ ... ] }
func SyncOfflineTxnsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			DeviceToken  string                     `json:"device_token" binding:"required"`
			Transactions []repository.OfflineTxn    `json:"transactions" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(in.Transactions) == 0 {
			c.JSON(http.StatusOK, gin.H{"results": []any{}})
			return
		}
		if len(in.Transactions) > 500 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Max 500 transactions per sync — send the queue in pages",
			})
			return
		}

		deviceID, vendorID, err := repository.ResolvePOSDevice(ctx, pool, eventID, in.DeviceToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unrecognised or suspended till"})
			return
		}

		// The policy the SERVER holds, not what the device claims. A till that
		// reports generous caps gets checked against the real ones.
		policy, err := repository.LoadOfflinePolicy(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load offline policy"})
			return
		}

		// Note this does NOT refuse the batch when offline spend is disabled.
		// The sales already happened — the till was offline and cannot be
		// argued with retrospectively. They are ingested and every one is
		// flagged by the recheck, which is the honest handling: record what
		// occurred, mark it wrong, let a human see it. Discarding them would
		// destroy the evidence and leave vendors unpaid for real drinks.
		results, err := repository.IngestOfflineTxns(ctx, pool, eventID, deviceID, vendorID,
			policy, in.Transactions, verifyOfflineSignature)
		if err != nil {
			log.Error().Err(err).Str("device_id", deviceID).
				Msg("offline sync: batch failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to ingest transactions"})
			return
		}

		var applied, flagged, dupes, rejected int
		for _, r := range results {
			switch r.Verdict {
			case repository.OfflineApplied:
				applied++
			case repository.OfflineFlagged:
				flagged++
			case repository.OfflineDuplicate:
				dupes++
			default:
				rejected++
			}
		}
		if flagged > 0 || rejected > 0 {
			log.Warn().Str("device_id", deviceID).Str("event_id", eventID).
				Int("flagged", flagged).Int("rejected", rejected).
				Msg("offline sync: anomalies in batch")
		}

		c.JSON(http.StatusOK, gin.H{
			"results": results,
			"summary": gin.H{
				"applied": applied, "flagged": flagged,
				"duplicate": dupes, "rejected": rejected,
			},
		})
	}
}

// verifyOfflineSignature adapts services.VerifyOfflineTxnSignature to the
// repository's seam.
//
// The adapter exists because repository cannot import services — services
// already imports repository, and Go rejects the cycle. Handlers can import
// both, so this is the right place for the join.
func verifyOfflineSignature(pubKeyB64, sigB64 string, in repository.OfflineSigInput) error {
	return services.VerifyOfflineTxnSignature(pubKeyB64, sigB64, services.OfflineTxnPayload{
		TxnID:        in.TxnID,
		EventID:      in.EventID,
		TagUID:       in.TagUID,
		VendorID:     in.VendorID,
		DeviceID:     in.DeviceID,
		AmountCents:  in.AmountCents,
		DeviceSeq:    in.DeviceSeq,
		AccumBefore:  in.AccumBefore,
		AccumAfter:   in.AccumAfter,
		ApprovedAtMs: in.ApprovedAtMs,
	})
}

// ── Mandate capture ──────────────────────────────────────────────────────────

// CreateMandateHandler records explicit consent to charge a card later.
//
//	POST /v1/cashless/events/:id/mandate
//	{ "authorization_code": "...", "max_charge": 200.00,
//	  "consent_text": "...", "consent_version": "v1" }
//
// ATTENDEE-FACING. The caller is the wallet holder giving permission, so this
// must not sit behind the staff check — a staff member cannot consent on
// somebody else's behalf, which is the entire point of a mandate.
//
// consent_text is stored VERBATIM as evidence. If an attendee later disputes a
// recovery charge, that row is the answer, and "we showed them something like
// this" is not a defence. The client must send exactly what was displayed.
func CreateMandateHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		eventID := c.Param("id")
		userID := c.MustGet("user_id").(string)

		var in struct {
			AuthorizationCode string  `json:"authorization_code" binding:"required"`
			CardLast4         string  `json:"card_last4"`
			CardBrand         string  `json:"card_brand"`
			MaxCharge         float64 `json:"max_charge" binding:"required,gt=0"`
			ConsentText       string  `json:"consent_text" binding:"required,min=20"`
			ConsentVersion    string  `json:"consent_version" binding:"required,max=32"`
			ExpiresInDays     int     `json:"expires_in_days"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		maxCents, err := repository.ToCents(in.MaxCharge)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid max_charge"})
			return
		}

		// A default rather than unbounded. An open-ended permission to charge
		// somebody's card is not something to create by omission.
		days := in.ExpiresInDays
		if days <= 0 || days > 365 {
			days = 90
		}

		wallet, err := repository.GetOrCreateWallet(ctx, pool, eventID, &userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open wallet"})
			return
		}

		id, err := repository.CreateMandate(ctx, pool, wallet.ID, eventID, userID,
			in.AuthorizationCode, in.CardLast4, in.CardBrand, maxCents,
			in.ConsentText, in.ConsentVersion, c.ClientIP(),
			time.Now().AddDate(0, 0, days))
		if err != nil {
			log.Error().Err(err).Str("wallet_id", wallet.ID).Msg("mandate: create failed")
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"mandate_id": id,
			"max_charge": in.MaxCharge,
			"expires_at": time.Now().AddDate(0, 0, days),
		})
	}
}

// RevokeMandateHandler withdraws consent.
//
//	POST /v1/cashless/events/:id/mandate/revoke
//
// Attendee-facing and deliberately unconditional: consent that cannot be
// withdrawn was never really consent, and an outstanding shortfall is not a
// reason to refuse. The balance remains owed; only the permission to take it
// automatically goes away.
func RevokeMandateHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := c.Param("id")
		userID := c.MustGet("user_id").(string)

		res, err := pool.Exec(ctx, `
			UPDATE payment_mandates
			SET status = 'revoked', revoked_at = now()
			WHERE event_id = $1 AND user_id = $2 AND status = 'active'
		`, eventID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke mandate"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "Mandate revoked. Any outstanding balance is still owed and " +
				"you will be asked to settle it directly.",
			"revoked": res.RowsAffected(),
		})
	}
}

// ListShortfallsHandler is the organiser's recovery view.
//
//	GET /v1/staff/events/:id/cashless/shortfalls
func ListShortfallsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		rows, err := pool.Query(ctx, `
			SELECT s.id, s.wallet_id, s.amount_cents, s.recovered_cents,
			       s.state::text, s.attempts, s.fraud_flag, s.detected_at, s.last_error
			FROM wallet_shortfalls s
			WHERE s.event_id = $1
			ORDER BY s.fraud_flag DESC, s.detected_at DESC
			LIMIT 200
		`, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load shortfalls"})
			return
		}
		defer rows.Close()

		out := []gin.H{}
		for rows.Next() {
			var (
				id, walletID, state string
				amount, recovered   int64
				attempts            int
				fraud               bool
				detected            time.Time
				lastErr             *string
			)
			if err := rows.Scan(&id, &walletID, &amount, &recovered, &state,
				&attempts, &fraud, &detected, &lastErr); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read shortfalls"})
				return
			}
			out = append(out, gin.H{
				"id": id, "wallet_id": walletID,
				"amount": repository.FromCents(amount),
				"recovered": repository.FromCents(recovered),
				"state": state, "attempts": attempts,
				// Sorted first and named plainly: this is not a debt to collect,
				// it is evidence that an offline cap was bypassed.
				"needs_investigation": fraud,
				"detected_at": detected, "last_error": lastErr,
			})
		}
		c.JSON(http.StatusOK, gin.H{"shortfalls": out})
	}
}