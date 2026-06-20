-- 000046_payments_booking_unique.up.sql
--
-- createPaymentRecord upserts with `ON CONFLICT (booking_id) DO UPDATE`, which
-- requires a UNIQUE constraint/index on payments.booking_id. The Stitch-era
-- payments table never had one, and migration 000044 didn't add it, so every
-- insert failed with:
--   ERROR: there is no unique or exclusion constraint matching the ON CONFLICT
--   specification (SQLSTATE 42P10)
-- No payment row was persisted, so GET /v1/bookings/:id/payment 404'd forever.
-- (The test schema already declared UNIQUE(booking_id); production lacked it.)

-- Defensive de-dup: the app writes exactly one payment per booking, but if any
-- booking somehow has multiple payment rows, keep the most recent so the unique
-- index can be built. NULL booking_ids are left alone (distinct under a unique index).
DELETE FROM payments p
USING payments q
WHERE p.booking_id IS NOT NULL
  AND p.booking_id = q.booking_id
  AND (p.created_at < q.created_at
       OR (p.created_at = q.created_at AND p.id < q.id));

-- Plain (non-partial) unique index so `ON CONFLICT (booking_id)` infers it.
CREATE UNIQUE INDEX IF NOT EXISTS payments_booking_id_uniq
    ON payments (booking_id);