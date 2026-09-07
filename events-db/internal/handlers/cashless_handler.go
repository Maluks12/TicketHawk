package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Cashless handlers (Phase 3: online) ───────────────────────────────────────
//
// Authorisation reuses requireStaffManager and requireCanScan rather than
// inventing a vendor permission axis for humans. A person who may work a gate
// may work a till; revoking them cuts off both at once, which is the property
// that matters when somebody walks off a shift mid-event.
//
// Till identity is separate and answers a different question — WHICH till rang
// this up — which a human credential cannot, and which vendor settlement needs.
// So a purchase requires BOTH: the operator's staff authorisation and a valid
// device token.
//
// Route shapes are all static under /cashless and /vendors, sidestepping
// static-vs-wildcard sibling conflicts in the router tree, as in the Phase 1
// tag handlers.

// ── Vendors ──────────────────────────────────────────────────────────────────

// CreateVendorHandler
//
//	POST /v1/staff/events/:id/vendors
//	{ "name": "Main Bar", "code": "BAR1", "commission_rate": 5.0 }
func CreateVendorHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			Name           string   `json:"name" binding:"required,max=160"`
			Code           string   `json:"code" binding:"required,max=24"`
			CommissionRate *float64 `json:"commission_rate"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if in.CommissionRate != nil && (*in.CommissionRate < 0 || *in.CommissionRate > 100) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_rate must be between 0 and 100"})
			return
		}

		v, err := repository.CreateVendor(ctx, pool, eventID, in.Name, in.Code,
			in.CommissionRate, c.MustGet("user_id").(string))
		if err != nil {
			// uq_vendor_event_code is the likely cause, and the code is printed
			// on the stall, so a clash is an operator decision not a server error.
			c.JSON(http.StatusConflict, gin.H{
				"error": "Could not create vendor — the code may already be in use at this event",
			})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"vendor": v})
	}
}

// ListVendorsHandler
//
//	GET /v1/staff/events/:id/vendors
func ListVendorsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}
		vendors, err := repository.ListVendors(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load vendors"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"vendors": vendors})
	}
}

// CreateVendorProductHandler adds one menu line.
//
//	POST /v1/staff/events/:id/vendor-products
//	{ "vendor_id": "...", "name": "Draught 500ml", "category": "Beer", "price": 45.00 }
func CreateVendorProductHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			VendorID  string  `json:"vendor_id" binding:"required,uuid"`
			Name      string  `json:"name" binding:"required,max=160"`
			Category  *string `json:"category"`
			Price     float64 `json:"price" binding:"gte=0"`
			SortOrder int     `json:"sort_order"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !vendorBelongsToEvent(ctx, pool, in.VendorID, eventID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Vendor not found for this event"})
			return
		}

		p, err := repository.UpsertVendorProduct(ctx, pool, in.VendorID, in.Name,
			in.Category, in.Price, in.SortOrder)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"product": p})
	}
}

// ── POS devices ──────────────────────────────────────────────────────────────

// ProvisionPOSDeviceHandler issues a till token.
//
//	POST /v1/staff/events/:id/pos-devices
//	{ "vendor_id": "...", "label": "Bar 1 — till 2" }
//
// The token is returned exactly once. Only its hash is stored, so a lost token
// means re-provisioning the till rather than looking it up.
func ProvisionPOSDeviceHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			VendorID string `json:"vendor_id" binding:"required,uuid"`
			Label    string `json:"label" binding:"required,max=120"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !vendorBelongsToEvent(ctx, pool, in.VendorID, eventID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Vendor not found for this event"})
			return
		}

		d, err := repository.ProvisionPOSDevice(ctx, pool, eventID, in.VendorID,
			in.Label, c.MustGet("user_id").(string))
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("cashless: provision POS failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to provision device"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"device":  d,
			"warning": "Store this token now — it is shown once and cannot be recovered.",
		})
	}
}

// ── Wallets ──────────────────────────────────────────────────────────────────

// CreateWalletHandler opens a wallet and optionally binds a band to it.
//
//	POST /v1/staff/events/:id/cashless/wallets
//	{ "user_id": "...", "tag_uid": "..." }     // both optional
//
// user_id omitted means a cash walk-up: an anonymous wallet with no account.
// Requiring one would exclude exactly the attendees a cashless event most risks
// excluding.
func CreateWalletHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			UserID string `json:"user_id"`
			TagUID string `json:"tag_uid"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var userID *string
		if in.UserID != "" {
			if _, err := uuid.Parse(in.UserID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user_id"})
				return
			}
			userID = &in.UserID
		}

		w, err := repository.GetOrCreateWallet(ctx, pool, eventID, userID)
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("cashless: wallet create failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create wallet"})
			return
		}

		if in.TagUID != "" {
			uid, err := services.NormaliseUID(in.TagUID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed tag UID"})
				return
			}
			if err := repository.BindWalletToTag(ctx, pool, eventID, uid, w.ID); err != nil {
				// The wallet exists either way — report the binding failure
				// without pretending the whole operation failed, or the operator
				// will retry and create confusion about duplicate wallets.
				c.JSON(http.StatusConflict, gin.H{
					"error":  err.Error(),
					"wallet": w,
				})
				return
			}
		}
		c.JSON(http.StatusCreated, gin.H{"wallet": w})
	}
}

// GetWalletByTagHandler resolves a band to its wallet and balance.
//
//	GET /v1/staff/events/:id/cashless/balance?tag_uid=...
//
// The till calls this on tap, before the cashier rings anything up.
func GetWalletByTagHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		uid, err := services.NormaliseUID(c.Query("tag_uid"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed or missing tag_uid"})
			return
		}

		target, err := repository.ResolveTagForSpend(ctx, pool, eventID, uid)
		if errors.Is(err, repository.ErrTagNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Band is not registered for this event"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve band"})
			return
		}
		if target.WalletID == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "This band is not linked to a wallet",
				"tag_uid": uid,
			})
			return
		}

		w, err := repository.GetWallet(ctx, pool, *target.WalletID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load wallet"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"wallet": w, "tag_uid": uid, "tag_status": target.Context.TagStatus})
	}
}

// GetWalletStatementHandler
//
//	GET /v1/staff/events/:id/cashless/statement?wallet_id=...&limit=100
func GetWalletStatementHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		walletID := c.Query("wallet_id")
		if _, err := uuid.Parse(walletID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "wallet_id is required"})
			return
		}

		w, err := repository.GetWallet(ctx, pool, walletID)
		if errors.Is(err, repository.ErrWalletNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load wallet"})
			return
		}
		// Scoped to the event the caller is authorised for. Without this, staff
		// at one event could read wallets from another by guessing an id.
		if w.EventID != eventID {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
			return
		}

		txns, err := repository.ListWalletTransactions(ctx, pool, walletID, 100)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load statement"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"wallet": w, "transactions": txns})
	}
}

// ── Top-ups ──────────────────────────────────────────────────────────────────

// CashTopupHandler takes notes at a booth and credits immediately.
//
//	POST /v1/staff/events/:id/cashless/topups/cash
//	{ "wallet_id": "...", "amount": 200.00, "fee": 2.00 }
//
// The fee is disclosed as a plain line item and stored on the top-up row, so a
// statement shows exactly what the attendee was told (§6). The v3.0 hidden-fee
// framing was removed from the architecture doc for good reason — a concealed
// fee generates "not as described" disputes, which 3DS liability shift does not
// cover because it only applies to fraud.
func CashTopupHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			WalletID string  `json:"wallet_id" binding:"required,uuid"`
			Amount   float64 `json:"amount" binding:"required,gt=0"`
			Fee      float64 `json:"fee" binding:"gte=0"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		amountCents, err := repository.ToCents(in.Amount)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount"})
			return
		}
		feeCents, err := repository.ToCents(in.Fee)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fee"})
			return
		}
		if feeCents >= amountCents {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Fee must be less than the amount — this would credit nothing",
			})
			return
		}

		txn, err := repository.CreateCashTopup(ctx, pool, eventID, in.WalletID,
			amountCents, feeCents, c.MustGet("user_id").(string))
		if err != nil {
			var refusal *repository.SpendRefusal
			if errors.As(err, &refusal) {
				c.JSON(http.StatusConflict, gin.H{"error": "Wallet is not active", "reason": refusal.Reason})
				return
			}
			if errors.Is(err, repository.ErrWalletNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
				return
			}
			log.Error().Err(err).Str("wallet_id", in.WalletID).Msg("cashless: cash topup failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record top-up"})
			return
		}

		w, _ := repository.GetWallet(ctx, pool, in.WalletID)
		c.JSON(http.StatusCreated, gin.H{
			"transaction": txn,
			"wallet":      w,
			"credited":    repository.FromCents(amountCents - feeCents),
			"fee":         in.Fee,
		})
	}
}

// CardTopupHandler starts a Paystack card top-up.
//
//	POST /v1/cashless/events/:id/topups/card
//	{ "amount": 200.00 }
//
// Attendee-facing: the caller is the wallet holder, not staff. Nothing is
// credited here — the webhook does that once Paystack confirms.
func CardTopupHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		eventID := c.Param("id")
		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event id"})
			return
		}
		userID := c.MustGet("user_id").(string)

		var in struct {
			Amount float64 `json:"amount" binding:"required,gt=0"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		amountCents, err := repository.ToCents(in.Amount)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount"})
			return
		}

		w, err := repository.GetOrCreateWallet(ctx, pool, eventID, &userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open wallet"})
			return
		}

		redirectURL, reference, err := paymentService.CreateCashlessTopupIntent(
			eventID, w.ID, userID, amountCents)
		if err != nil {
			log.Error().Err(err).Str("wallet_id", w.ID).Msg("cashless: card topup intent failed")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Payment service temporarily unavailable. Please try again shortly.",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"authorization_url": redirectURL,
			"reference":         reference,
			"amount":            in.Amount,
			"currency":          models.DefaultCurrency,
			"wallet_id":         w.ID,
		})
	}
}

// ── The purchase ─────────────────────────────────────────────────────────────

// PurchaseHandler rings up a sale against a band's wallet.
//
//	POST /v1/staff/events/:id/cashless/purchase
//	{ "txn_id": "<uuid>", "device_token": "...", "tag_uid": "...",
//	  "amount": 87.50, "items": [{ "name": "Draught 500ml", "qty": 2, "price": 45.00 }] }
//
// txn_id is CLIENT-generated, exactly like a scan id: a till that loses its
// response and retries must not charge twice, and letting the client name the
// transaction is the only reliable way to guarantee that. Repeating a known id
// returns the original sale and writes nothing.
//
// The amount is authoritative; items are a receipt, kept verbatim because a
// menu changes during the night and a statement must show what was actually
// sold rather than what the product row says today.
func PurchaseHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}
		operatorID := c.MustGet("user_id").(string)

		var in struct {
			TxnID       string           `json:"txn_id" binding:"required,uuid"`
			DeviceToken string           `json:"device_token" binding:"required"`
			TagUID      string           `json:"tag_uid" binding:"required"`
			Amount      float64          `json:"amount" binding:"required,gt=0"`
			Items       []map[string]any `json:"items"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Till identity. The operator's staff authorisation was already checked
		// above; this answers the separate question of which till rang it up,
		// which vendor settlement needs and a human credential cannot supply.
		deviceID, vendorID, err := repository.ResolvePOSDevice(ctx, pool, eventID, in.DeviceToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unrecognised or suspended till"})
			return
		}

		uid, err := services.NormaliseUID(in.TagUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed tag UID"})
			return
		}
		amountCents, err := repository.ToCents(in.Amount)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount"})
			return
		}

		detail := map[string]any{}
		if len(in.Items) > 0 {
			detail["items"] = in.Items
		}

		txn, err := repository.RecordPurchase(ctx, pool, eventID, in.TxnID, uid,
			vendorID, deviceID, amountCents, detail, operatorID)
		if err != nil {
			var refusal *repository.SpendRefusal
			if errors.As(err, &refusal) {
				// 402 rather than 409: the request was well-formed and the band
				// is real, there just is not enough money. The till shows the
				// balance so the cashier can offer a partial or a top-up.
				status := http.StatusConflict
				if refusal.Reason == repository.SpendInsufficient {
					status = http.StatusPaymentRequired
				}
				c.JSON(status, gin.H{
					"error":   refusalMessage(refusal.Reason),
					"reason":  refusal.Reason,
					"balance": refusal.Balance,
				})
				return
			}
			switch {
			case errors.Is(err, repository.ErrTagNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "Band is not registered for this event"})
			case errors.Is(err, repository.ErrTagHasNoWallet):
				c.JSON(http.StatusNotFound, gin.H{"error": "This band is not linked to a wallet"})
			default:
				log.Error().Err(err).Str("tag_uid", uid).Str("event_id", eventID).
					Msg("cashless: purchase failed")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record purchase"})
			}
			return
		}

		w, _ := repository.GetWallet(ctx, pool, txn.WalletID)
		c.JSON(http.StatusCreated, gin.H{
			"transaction": txn,
			"balance":     w.Balance,
		})
	}
}

// refusalMessage turns a machine reason into something a cashier with a queue
// behind them can act on. Each maps to a different thing they should do next.
func refusalMessage(reason string) string {
	switch reason {
	case repository.SpendInsufficient:
		return "Not enough on the band — offer a top-up or a smaller order"
	case repository.SpendTagNotActive:
		return "This band is not active — send them to the help desk"
	case repository.SpendNoWallet:
		return "This band has no wallet linked"
	case repository.SpendWalletNotActive:
		return "This wallet is frozen — send them to the help desk"
	case repository.SpendChipNotEligible:
		return "This band's chip cannot hold a wallet"
	case repository.SpendAmountTooLarge:
		return "That amount looks wrong — check the decimal point"
	default:
		return "Payment refused"
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func vendorBelongsToEvent(ctx context.Context, pool *pgxpool.Pool, vendorID, eventID string) bool {
	var ok bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM event_vendors WHERE id = $1 AND event_id = $2)`,
		vendorID, eventID).Scan(&ok); err != nil {
		return false
	}
	return ok
}