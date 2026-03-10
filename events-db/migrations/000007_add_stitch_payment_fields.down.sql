-- migrations/000007_add_stitch_payment_fields.down.sql

ALTER TABLE payments
    DROP COLUMN IF EXISTS stitch_payment_request_id,
    DROP COLUMN IF EXISTS stitch_payment_url,
    DROP COLUMN IF EXISTS stitch_linked_account_id;

ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS uq_payments_booking_id;

DROP INDEX IF EXISTS idx_payments_stitch_request_id;
