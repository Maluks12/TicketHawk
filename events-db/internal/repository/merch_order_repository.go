package repository

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// FulfillMerchOrders is called by the payment webhook when a payment is confirmed.
// It atomically:
//  1. Fetches all merchandise cart_items for the cart, including fulfillment data.
//  2. For each item, attempts to decrement stock and insert a merch_order row.
//  3. If stock is insufficient for an item, the order is still recorded with
//     fulfillment_status = 'oversold' so the admin can resolve it.
//
// The unique constraint on (stitch_payment_req_id, variant_id) makes this
// safe to call multiple times — duplicate webhook deliveries are no-ops.
func FulfillMerchOrders(
	ctx context.Context,
	pool *pgxpool.Pool,
	cartID string,
	stitchPaymentReqID string,
) error {
	// Idempotency fast-path: if any order already exists for this payment + cart,
	// the webhook has already been processed.
	var existingCount int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM merch_orders
		WHERE stitch_payment_req_id = $1
		  AND cart_id = $2
	`, stitchPaymentReqID, cartID).Scan(&existingCount)
	if err != nil {
		return fmt.Errorf("FulfillMerchOrders: idempotency check: %w", err)
	}
	if existingCount > 0 {
		log.Info().
			Str("cart_id", cartID).
			Str("stitch_id", stitchPaymentReqID).
			Msg("merch fulfillment: already processed, skipping")
		return nil
	}

	// Fetch all merch items in this cart including fulfillment choice and the
	// merchandise's pickup_location (snapshotted into the order record).
	type cartMerchItem struct {
		variantID         string
		merchandiseID     string
		eventID           string
		quantity          int
		unitPrice         float64
		fulfillmentMethod *string
		deliveryAddress   *string // JSONB text
		pickupLocation    *string // from merchandise table
	}

	rows, err := pool.Query(ctx, `
		SELECT
			ci.merchandise_variant_id,
			mv.merchandise_id,
			ci.event_id,
			ci.quantity,
			ci.unit_price_snapshot,
			ci.fulfillment_method,
			ci.delivery_address::text,
			m.pickup_location
		FROM cart_items ci
		JOIN merchandise_variants mv ON mv.id = ci.merchandise_variant_id
		JOIN merchandise m            ON m.id  = mv.merchandise_id
		WHERE ci.cart_id   = $1
		  AND ci.item_type = 'merchandise'
	`, cartID)
	if err != nil {
		return fmt.Errorf("FulfillMerchOrders: fetch cart items: %w", err)
	}

	var items []cartMerchItem
	for rows.Next() {
		var item cartMerchItem
		if err := rows.Scan(
			&item.variantID,
			&item.merchandiseID,
			&item.eventID,
			&item.quantity,
			&item.unitPrice,
			&item.fulfillmentMethod,
			&item.deliveryAddress,
			&item.pickupLocation,
		); err != nil {
			rows.Close()
			return fmt.Errorf("FulfillMerchOrders: scan cart item: %w", err)
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("FulfillMerchOrders: row iteration: %w", err)
	}

	if len(items) == 0 {
		return nil // Cart had no merch items — nothing to do.
	}

	// Fetch cart owner for order attribution.
	var userID *string
	var sessionID *string
	pool.QueryRow(ctx, `SELECT user_id, session_id FROM carts WHERE id = $1`, cartID).
		Scan(&userID, &sessionID)

	// Process each merch item in its own transaction to maximise partial success.
	for _, item := range items {
		if err := fulfillSingleMerchItem(
			ctx, pool,
			item.variantID, item.merchandiseID, item.eventID,
			cartID, stitchPaymentReqID,
			userID, sessionID,
			item.quantity, item.unitPrice,
			item.fulfillmentMethod, item.deliveryAddress, item.pickupLocation,
		); err != nil {
			log.Error().Err(err).
				Str("variant_id", item.variantID).
				Str("cart_id", cartID).
				Msg("merch fulfillment: failed to process item")
		}
	}

	return nil
}

// fulfillSingleMerchItem handles one cart_item in its own transaction.
// It tries to decrement stock; if stock is insufficient it inserts an
// 'oversold' order record so the admin can take action.
func fulfillSingleMerchItem(
	ctx context.Context,
	pool *pgxpool.Pool,
	variantID, merchandiseID, eventID, cartID, stitchPaymentReqID string,
	userID, sessionID *string,
	quantity int,
	unitPrice float64,
	fulfillmentMethod *string,
	deliveryAddress *string,
	pickupLocation *string,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	totalAmount := unitPrice * float64(quantity)

	// Attempt to decrement stock atomically.
	cmd, err := tx.Exec(ctx, `
		UPDATE merchandise_variants
		SET stock_quantity = stock_quantity - $1,
		    updated_at     = NOW()
		WHERE id = $2
		  AND stock_quantity >= $1
	`, quantity, variantID)
	if err != nil {
		return fmt.Errorf("decrement stock for variant %s: %w", variantID, err)
	}

	fulfillmentStatus := models.FulfillmentFulfilled
	if cmd.RowsAffected() == 0 {
		fulfillmentStatus = models.FulfillmentOversold
		log.Warn().
			Str("variant_id", variantID).
			Str("cart_id", cartID).
			Int("quantity_requested", quantity).
			Msg("merch fulfillment: OVERSOLD — payment captured but stock exhausted; manual resolution required")
	}

	// Insert the order record, snapshotting fulfillment details.
	// ON CONFLICT DO NOTHING provides idempotency on duplicate webhook calls.
	var deliveryAddrJSONB interface{}
	if deliveryAddress != nil {
		deliveryAddrJSONB = *deliveryAddress
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO merch_orders (
			cart_id, merchandise_id, variant_id, event_id,
			user_id, session_id,
			quantity, unit_price, total_amount,
			stitch_payment_req_id, fulfillment_status,
			fulfillment_method, delivery_address, pickup_location
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14)
		ON CONFLICT (stitch_payment_req_id, variant_id) DO NOTHING
	`,
		cartID, merchandiseID, variantID, eventID,
		userID, sessionID,
		quantity, unitPrice, totalAmount,
		stitchPaymentReqID, string(fulfillmentStatus),
		fulfillmentMethod, deliveryAddrJSONB, pickupLocation,
	)
	if err != nil {
		return fmt.Errorf("insert merch_order for variant %s: %w", variantID, err)
	}

	return tx.Commit(ctx)
}

// GetMerchOrdersForUser returns a user's merch purchase history, newest first.
func GetMerchOrdersForUser(pool *pgxpool.Pool, userID string, limit int) ([]models.MerchOrder, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
			mo.id, mo.cart_id, mo.merchandise_id, mo.variant_id, mo.event_id,
			mo.user_id, mo.session_id,
			mo.quantity, mo.unit_price, mo.total_amount,
			mo.stitch_payment_req_id, mo.fulfillment_status,
			mo.fulfillment_method,
			mo.delivery_address::text,
			mo.pickup_location,
			mo.created_at, mo.updated_at,
			m.name  AS merch_name,
			mv.sku  AS variant_sku,
			e.title AS event_title
		FROM merch_orders mo
		JOIN merchandise m ON m.id = mo.merchandise_id
		JOIN merchandise_variants mv ON mv.id = mo.variant_id
		JOIN events e ON e.id = mo.event_id
		WHERE mo.user_id = $1
		ORDER BY mo.created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetMerchOrdersForUser: %w", err)
	}
	defer rows.Close()

	var orders []models.MerchOrder
	for rows.Next() {
		var o models.MerchOrder
		if err := rows.Scan(
			&o.ID, &o.CartID, &o.MerchandiseID, &o.VariantID, &o.EventID,
			&o.UserID, &o.SessionID,
			&o.Quantity, &o.UnitPrice, &o.TotalAmount,
			&o.StitchPaymentReqID, &o.FulfillmentStatus,
			&o.FulfillmentMethod,
			&o.DeliveryAddress,
			&o.PickupLocation,
			&o.CreatedAt, &o.UpdatedAt,
			&o.MerchName, &o.VariantSKU, &o.EventTitle,
		); err != nil {
			return nil, fmt.Errorf("GetMerchOrdersForUser scan: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

// GetOversoldMerchOrders returns all oversold merch orders for admin review.
func GetOversoldMerchOrders(pool *pgxpool.Pool) ([]models.MerchOrder, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
			mo.id, mo.cart_id, mo.merchandise_id, mo.variant_id, mo.event_id,
			mo.user_id, mo.session_id,
			mo.quantity, mo.unit_price, mo.total_amount,
			mo.stitch_payment_req_id, mo.fulfillment_status,
			mo.fulfillment_method,
			mo.delivery_address::text,
			mo.pickup_location,
			mo.created_at, mo.updated_at,
			m.name  AS merch_name,
			mv.sku  AS variant_sku,
			e.title AS event_title
		FROM merch_orders mo
		JOIN merchandise m ON m.id = mo.merchandise_id
		JOIN merchandise_variants mv ON mv.id = mo.variant_id
		JOIN events e ON e.id = mo.event_id
		WHERE mo.fulfillment_status = 'oversold'
		ORDER BY mo.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("GetOversoldMerchOrders: %w", err)
	}
	defer rows.Close()

	var orders []models.MerchOrder
	for rows.Next() {
		var o models.MerchOrder
		if err := rows.Scan(
			&o.ID, &o.CartID, &o.MerchandiseID, &o.VariantID, &o.EventID,
			&o.UserID, &o.SessionID,
			&o.Quantity, &o.UnitPrice, &o.TotalAmount,
			&o.StitchPaymentReqID, &o.FulfillmentStatus,
			&o.FulfillmentMethod,
			&o.DeliveryAddress,
			&o.PickupLocation,
			&o.CreatedAt, &o.UpdatedAt,
			&o.MerchName, &o.VariantSKU, &o.EventTitle,
		); err != nil {
			return nil, fmt.Errorf("GetOversoldMerchOrders scan: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}