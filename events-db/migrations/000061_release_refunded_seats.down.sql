-- migrations/000061_release_refunded_seats.down.sql
--
-- Dropping the marker reintroduces the double-release risk, because the rolled
-- back code has no way to tell whether a booking already returned its seats.
-- The tickets_sold corrections applied by the up migration are NOT reverted —
-- they made the counters match reality, and undoing that would only restore the
-- drift.

DROP INDEX IF EXISTS idx_bookings_seats_not_released;

ALTER TABLE bookings
    DROP COLUMN IF EXISTS seats_released_at;