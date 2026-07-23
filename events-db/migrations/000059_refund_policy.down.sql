-- migrations/000058_refund_policy.down.sql
--
-- Note: dropping refund_amount destroys the record of how much each buyer was
-- actually given back. Bookings will still read 'refunded', and the settlement
-- query in the rolled-back code treats every refunded booking as fully refunded,
-- so no organiser gets overpaid — but partial-refund history is not recoverable.

DROP INDEX IF EXISTS idx_bookings_refunded_event;

ALTER TABLE bookings
    DROP CONSTRAINT IF EXISTS chk_refund_amount_range;

ALTER TABLE bookings
    DROP COLUMN IF EXISTS refund_amount,
    DROP COLUMN IF EXISTS refund_rate,
    DROP COLUMN IF EXISTS refunded_at;