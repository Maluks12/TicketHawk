-- migrations/000038_listing_fee.down.sql

DROP INDEX IF EXISTS idx_events_listing_fee_paid;
DROP INDEX IF EXISTS idx_events_listing_fee_request_id;

ALTER TABLE events
    DROP COLUMN IF EXISTS listing_fee_payment_request_id,
    DROP COLUMN IF EXISTS listing_fee_amount,
    DROP COLUMN IF EXISTS listing_fee_paid;