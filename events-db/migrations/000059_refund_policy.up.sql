-- migrations/000058_refund_policy.up.sql
--
-- Sliding-scale refunds.
--
-- Until now a refund was all-or-nothing: bookings.booking_status flipped to
-- 'refunded' and payments.refunded_amount was set to the full payment amount.
-- The new policy returns a percentage of SUBTOTAL that depends on how far the
-- event still is when the refund is requested, so the settlement ledger has to
-- know how much of each booking was actually given back.
--
-- We do NOT add a 'partially_refunded' booking status. A partial refund still
-- voids the tickets completely — the buyer simply gets less money back — so
-- 'refunded' remains the correct state and refund_amount carries the nuance.
-- That avoids an enum change and keeps every existing query valid.

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS refund_amount   DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS refund_rate     DECIMAL(5,4),
    ADD COLUMN IF NOT EXISTS refunded_at     TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN bookings.refund_amount IS
    'ZAR actually returned to the buyer. Excludes platform_fee for buyer-initiated '
    'refunds; includes it for organiser cancellations. NULL means never refunded.';

COMMENT ON COLUMN bookings.refund_rate IS
    'Fraction of subtotal returned (0.0000-1.0000), recorded at refund time so a '
    'later change to the schedule never rewrites history.';

-- refund_amount must be sane relative to what the buyer actually paid.
-- Cancellations return subtotal + platform_fee, so total_amount is the ceiling.
ALTER TABLE bookings
    DROP CONSTRAINT IF EXISTS chk_refund_amount_range;
ALTER TABLE bookings
    ADD CONSTRAINT chk_refund_amount_range CHECK (
        refund_amount IS NULL
        OR (refund_amount >= 0 AND refund_amount <= total_amount)
    );

-- Backfill: any booking already sitting at 'refunded' predates this migration
-- and was, under the old rules, refunded in full. Recording that explicitly
-- matters because the settlement query subtracts refund_amount — leaving these
-- NULL would make the ledger pay organisers for tickets that were given back.
UPDATE bookings
SET refund_amount = total_amount,
    refund_rate   = 1.0000,
    refunded_at   = COALESCE(refund_requested_at, updated_at, created_at)
WHERE booking_status = 'refunded'
  AND refund_amount IS NULL;

-- Partial index: the settlement and reserve-release queries filter on refunded
-- bookings per event, and refunds are a small minority of rows.
CREATE INDEX IF NOT EXISTS idx_bookings_refunded_event
    ON bookings (event_id)
    WHERE refund_amount IS NOT NULL;