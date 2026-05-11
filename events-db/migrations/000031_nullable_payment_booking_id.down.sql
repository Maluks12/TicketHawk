-- Revert: make booking_id NOT NULL again.
-- Rows with NULL booking_id (merch-only payments) must be deleted first.
DELETE FROM payments WHERE booking_id IS NULL;

DROP INDEX IF EXISTS uq_payments_booking_id_notnull;

ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_booking_id_fkey;
ALTER TABLE payments
    ADD CONSTRAINT payments_booking_id_fkey
    FOREIGN KEY (booking_id) REFERENCES bookings(id) ON DELETE RESTRICT;

ALTER TABLE payments ALTER COLUMN booking_id SET NOT NULL;

ALTER TABLE payments ADD CONSTRAINT uq_payments_booking_id UNIQUE (booking_id);