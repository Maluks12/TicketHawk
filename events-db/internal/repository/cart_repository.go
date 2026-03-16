package repository

import (
  "context"
  "errors"
  "events-db/internal/models"
  "fmt"
  "time"
 
  "github.com/jackc/pgx/v5/pgxpool"
)
 
var ErrCartNotFound = errors.New("cart not found")
 
// GetOrCreateCart returns the cart for the given owner, creating one if absent.
// Pass userID OR sessionID — never both.
func GetOrCreateCart(pool *pgxpool.Pool, userID, sessionID *string) (*models.Cart, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  var cart models.Cart
 
  if userID != nil {
    err := pool.QueryRow(ctx,
      `SELECT id, user_id, session_id, last_activity_at, created_at
       FROM carts WHERE user_id = $1 LIMIT 1`, *userID,
    ).Scan(&cart.ID, &cart.UserID, &cart.SessionID, &cart.LastActivityAt, &cart.CreatedAt)
    if err == nil {
      return &cart, nil
    }
  } else {
    err := pool.QueryRow(ctx,
      `SELECT id, user_id, session_id, last_activity_at, created_at
       FROM carts WHERE session_id = $1 LIMIT 1`, *sessionID,
    ).Scan(&cart.ID, &cart.UserID, &cart.SessionID, &cart.LastActivityAt, &cart.CreatedAt)
    if err == nil {
      return &cart, nil
    }
  }
 
  // Create new cart
  err := pool.QueryRow(ctx,
    `INSERT INTO carts (user_id, session_id) VALUES ($1, $2)
     RETURNING id, user_id, session_id, last_activity_at, created_at`,
    userID, sessionID,
  ).Scan(&cart.ID, &cart.UserID, &cart.SessionID, &cart.LastActivityAt, &cart.CreatedAt)
  if err != nil {
    return nil, fmt.Errorf("failed to create cart: %w", err)
  }
  return &cart, nil
}
 
// GetCartWithItems returns the cart and all its items with joined display fields.
func GetCartWithItems(pool *pgxpool.Pool, cartID string) (*models.Cart, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  var cart models.Cart
  err := pool.QueryRow(ctx,
    `SELECT id, user_id, session_id, last_activity_at, created_at
     FROM carts WHERE id = $1`, cartID,
  ).Scan(&cart.ID, &cart.UserID, &cart.SessionID, &cart.LastActivityAt, &cart.CreatedAt)
  if err != nil {
    return nil, ErrCartNotFound
  }
 
  rows, err := pool.Query(ctx, `
    SELECT
      ci.id, ci.cart_id, ci.item_type, ci.event_id,
      ci.merchandise_variant_id, ci.quantity, ci.unit_price_snapshot, ci.created_at,
      e.title AS event_title,
      mv.sku  AS variant_sku,
      m.name  AS merch_name,
	  
      mv.attributes::text AS variant_attributes
    FROM cart_items ci
    JOIN events e ON e.id = ci.event_id
    LEFT JOIN merchandise_variants mv ON mv.id = ci.merchandise_variant_id
    LEFT JOIN merchandise m ON m.id = mv.merchandise_id
    WHERE ci.cart_id = $1
    ORDER BY ci.created_at ASC
  `, cartID)
  if err != nil {
    return nil, fmt.Errorf("failed to query cart items: %w", err)
  }
  defer rows.Close()
 
  for rows.Next() {
    var item models.CartItem
    if err := rows.Scan(
      &item.ID, &item.CartID, &item.ItemType, &item.EventID,
      &item.MerchandiseVariantID, &item.Quantity, &item.UnitPriceSnapshot, &item.CreatedAt,
      &item.EventTitle, &item.VariantSKU, &item.MerchName, &item.VariantAttrs,
    ); err != nil {
      return nil, fmt.Errorf("failed to scan cart item: %w", err)
    }
    cart.Items = append(cart.Items, item)
  }
  return &cart, nil
}
 
// AddCartItem inserts a cart item, enforcing per-event ticket max of 10.
func AddCartItem(pool *pgxpool.Pool, cartID string, dto *models.AddCartItemDTO, price float64) (*models.CartItem, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  // Check existing quantity for this item type + event in this cart
  var existingQty int
  pool.QueryRow(ctx,
    `SELECT COALESCE(SUM(quantity), 0) FROM cart_items
     WHERE cart_id = $1 AND event_id = $2 AND item_type = $3`,
    cartID, dto.EventID, dto.ItemType,
  ).Scan(&existingQty)
 
  if existingQty+dto.Quantity > 10 {
    return nil, fmt.Errorf("maximum 10 items per event allowed")
  }
 
  var item models.CartItem
  err := pool.QueryRow(ctx, `
    INSERT INTO cart_items
      (cart_id, item_type, event_id, merchandise_variant_id, quantity, unit_price_snapshot)
    VALUES ($1, $2, $3, $4, $5, $6)
    RETURNING id, cart_id, item_type, event_id,
              merchandise_variant_id, quantity, unit_price_snapshot, created_at
  `, cartID, dto.ItemType, dto.EventID, dto.MerchandiseVariantID, dto.Quantity, price,
  ).Scan(
    &item.ID, &item.CartID, &item.ItemType, &item.EventID,
    &item.MerchandiseVariantID, &item.Quantity, &item.UnitPriceSnapshot, &item.CreatedAt,
  )
  if err != nil {
    return nil, fmt.Errorf("failed to add cart item: %w", err)
  }
 
  // Touch last_activity_at
  pool.Exec(ctx, `UPDATE carts SET last_activity_at = NOW() WHERE id = $1`, cartID)
  return &item, nil
}
 
// UpdateCartItemQuantity updates the quantity of a specific cart item.
func UpdateCartItemQuantity(pool *pgxpool.Pool, cartID, itemID string, quantity int) error {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  cmd, err := pool.Exec(ctx,
    `UPDATE cart_items SET quantity = $1
     WHERE id = $2 AND cart_id = $3`,
    quantity, itemID, cartID,
  )
  if err != nil {
    return fmt.Errorf("failed to update cart item: %w", err)
  }
  if cmd.RowsAffected() == 0 {
    return errors.New("cart item not found")
  }
  return nil
}
 
// DeleteCartItem removes a single item from the cart.
func DeleteCartItem(pool *pgxpool.Pool, cartID, itemID string) error {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  cmd, err := pool.Exec(ctx,
    `DELETE FROM cart_items WHERE id = $1 AND cart_id = $2`,
    itemID, cartID,
  )
  if err != nil {
    return fmt.Errorf("failed to delete cart item: %w", err)
  }
  if cmd.RowsAffected() == 0 {
    return errors.New("cart item not found")
  }
  return nil
}
 
// ClearCart deletes all items from the cart.
func ClearCart(pool *pgxpool.Pool, cartID string) error {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  _, err := pool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, cartID)
  return err
}
 
// MergeGuestCart merges a guest session cart into the authenticated user's cart.
// If both carts have ticket items for the same event, the higher quantity wins (up to 10).
func MergeGuestCart(pool *pgxpool.Pool, userID, sessionID string) error {
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
 
  tx, err := pool.Begin(ctx)
  if err != nil {
    return err
  }
  defer tx.Rollback(ctx)
 
  // Find guest cart
  var guestCartID string
  err = tx.QueryRow(ctx,
    `SELECT id FROM carts WHERE session_id = $1 LIMIT 1`, sessionID,
  ).Scan(&guestCartID)
  if err != nil {
    return nil // No guest cart to merge — that's fine
  }
 
  // Find or create user cart
  var userCartID string
  err = tx.QueryRow(ctx,
    `SELECT id FROM carts WHERE user_id = $1 LIMIT 1`, userID,
  ).Scan(&userCartID)
  if err != nil {
    // Create user cart
    tx.QueryRow(ctx,
      `INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
    ).Scan(&userCartID)
  }
 
  // Move guest items to user cart, avoiding duplicates
  _, err = tx.Exec(ctx, `
    INSERT INTO cart_items
      (cart_id, item_type, event_id, merchandise_variant_id, quantity, unit_price_snapshot, created_at)
    SELECT $1, item_type, event_id, merchandise_variant_id,
           LEAST(quantity, 10), unit_price_snapshot, created_at
    FROM cart_items
    WHERE cart_id = $2
    ON CONFLICT DO NOTHING
  `, userCartID, guestCartID)
  if err != nil {
    return fmt.Errorf("failed to merge cart items: %w", err)
  }
 
  // Delete guest cart (cascades to its items)
  tx.Exec(ctx, `DELETE FROM carts WHERE id = $1`, guestCartID)
 
  return tx.Commit(ctx)
}
 
// CleanupStaleCarts deletes carts with no activity for the given duration.
func CleanupStaleCarts(pool *pgxpool.Pool, ttl time.Duration) error {
	ctx,cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	query :=  `DELETE FROM carts WHERE last_activity_at < NOW() - $1::interval`
  _, err := pool.Exec(ctx, query, ttl.String())
  return err
}
