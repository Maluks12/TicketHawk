-- migrations/000038_listing_fee.up.sql
--
-- Listing Fee: organizers running FREE events (price = 0) with 20+ ticket
-- capacity must pay R5 per ticket upfront before their event appears in
-- any home-feed algorithm section. The organizer absorbs this cost —
-- ticket holders are never charged.
--
-- Paid-ticket events (price > 0) are NOT affected by this migration.
-- They continue to appear in the home feed without any listing fee.
--
-- Free events below the 20-ticket capacity threshold are published at no
-- cost but are also excluded from all home-feed sections (they remain
-- discoverable via search and direct link).
--
-- Ad campaigns (the existing system) are unaffected and continue to work
-- independently. An organizer running an ad campaign on a free event will
-- still see their sponsored card injected into feeds via the existing
-- InjectPreFetched path, regardless of listing_fee_paid status.

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS listing_fee_paid               BOOLEAN       NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS listing_fee_amount             DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS listing_fee_payment_request_id TEXT;

-- Fast webhook lookup: resolve a Stitch payment request ID → event
CREATE INDEX IF NOT EXISTS idx_events_listing_fee_request_id
    ON events (listing_fee_payment_request_id)
    WHERE listing_fee_payment_request_id IS NOT NULL;

-- Fast home-feed filter: partial index only over free, paid, published events
CREATE INDEX IF NOT EXISTS idx_events_free_listing_fee_paid
    ON events (listing_fee_paid, status, start_time)
    WHERE price = 0 AND listing_fee_paid = TRUE;

COMMENT ON COLUMN events.listing_fee_paid IS
    'TRUE once the organizer has paid the platform listing fee (R5 × capacity). '
    'Only relevant for free events (price = 0) with capacity >= 20. '
    'Paid-ticket events ignore this flag entirely.';

COMMENT ON COLUMN events.listing_fee_amount IS
    'The listing fee amount in ZAR stored at payment-initiation time. '
    '0 for paid-ticket events and free events below the 20-capacity threshold.';

COMMENT ON COLUMN events.listing_fee_payment_request_id IS
    'Stitch payment request ID for the listing fee charge. '
    'Set before the Stitch payment flow starts; webhook uses it to confirm payment.';