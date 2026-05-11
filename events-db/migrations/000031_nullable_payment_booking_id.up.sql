-- Allow payments to exist without a booking row, so merch-only carts can be
-- paid for without creating a ticket booking. Ticket-only and mixed carts are
-- unaffected — they still produce a booking before the payment is created.
--
-- Changes:
--   1. Drop NOT NULL on payments.booking_id
--   2. Re-add the FK as SET NULL on delete (so the payment survives if a booking
--      is deleted during cleanup)
--   3. Drop the UNIQUE constraint on booking_id because PostgreSQL treats each
--      NULL as distinct, but the old DO-block-guarded constraint still exists;
--      we replace it with a partial unique index that only enforces uniqueness
--      for non-NULL booking IDs, which is the semantics we want.

-- 1. Drop NOT NULL
ALTER TABLE payments ALTER COLUMN booking_id DROP NOT NULL;

-- 2. Update FK constraint
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_booking_id_fkey;
ALTER TABLE payments
    ADD CONSTRAINT payments_booking_id_fkey
    FOREIGN KEY (booking_id) REFERENCES bookings(id) ON DELETE SET NULL;

-- 3. Replace full UNIQUE constraint with a partial unique index
ALTER TABLE payments DROP CONSTRAINT IF EXISTS uq_payments_booking_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_booking_id_notnull
    ON payments (booking_id)
    WHERE booking_id IS NOT NULL;