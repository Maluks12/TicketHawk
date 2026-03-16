DROP INDEX IF EXISTS idx_payments_cart_id;
ALTER TABLE payments  DROP COLUMN IF EXISTS cart_id;

DROP INDEX IF EXISTS idx_bookings_cart_id;
ALTER TABLE bookings  DROP COLUMN IF EXISTS cart_id;