-- The platform fee split changed: the BUYER pays the per-ticket service fee
-- (platform_fee), and the ORGANIZER absorbs the commission (organizer_fee).
-- The organizer's commission is deducted from their payout — it is NOT charged
-- to the buyer — so it must not be part of total_amount.
--
-- The original chk_total_calculation required organizer_fee to be INCLUDED in
-- total_amount. Now that organizer_fee is a non-zero commission, that check
-- rejects every ticket booking ("failed to create booking"). Redefine it:
--
--   • ticket bookings : total_amount = subtotal + platform_fee − discount_amount
--   • merch-only      : exempt — the merch total is stored directly in
--                       total_amount and is not derived from these columns
--                       (subtotal/platform_fee are 0 for merch-only rows).
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS chk_total_calculation;

ALTER TABLE bookings
    ADD CONSTRAINT chk_total_calculation CHECK (
        booking_type = 'merch_only'
        OR total_amount = subtotal + platform_fee - COALESCE(discount_amount, 0)
    );