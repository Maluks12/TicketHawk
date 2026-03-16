-- Add cart_id to bookings so every booking created from a cart checkout
-- can be traced back to the originating cart.
ALTER TABLE bookings
  ADD COLUMN IF NOT EXISTS cart_id UUID REFERENCES carts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_bookings_cart_id
  ON bookings(cart_id) WHERE cart_id IS NOT NULL;

-- Add cart_id to payments so the webhook handler can confirm ALL bookings
-- that belong to a single cart payment in one go.
-- booking_id remains NOT NULL and linked to the PRIMARY booking; cart_id
-- is set alongside it for multi-booking (cart-based) checkouts.
ALTER TABLE payments
  ADD COLUMN IF NOT EXISTS cart_id UUID REFERENCES carts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_payments_cart_id
  ON payments(cart_id) WHERE cart_id IS NOT NULL;