-- migrations/000061_release_refunded_seats.up.sql
--
-- Refunded tickets never went back on sale.
--
-- events.tickets_sold was incremented when a booking was created and decremented
-- in exactly one place: the expiry worker, when a pending booking timed out.
-- Nothing decremented it on a refund, a guest refund, or an event cancellation.
-- repository.DecrementTicketsSold existed but was never called from anywhere.
--
-- The effect was slow and invisible: every refund permanently consumed a seat,
-- so an event with 100 seats and 20 refunds over its sale period showed sold out
-- at 80 real attendees, with nothing in the UI to explain why.
--
-- ── The marker ───────────────────────────────────────────────────────────────
--
-- Releasing seats must happen exactly once per booking. The refund path can be
-- retried (a gateway timeout, a re-run cancellation sweep), and a second release
-- would drive tickets_sold below the true figure and oversell the event — the
-- opposite failure, and a worse one.
--
-- seats_released_at is that anchor: the release is gated on it being NULL and
-- sets it in the same transaction, so concurrent or repeated attempts collapse
-- to a single decrement.

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS seats_released_at TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN bookings.seats_released_at IS
    'Set when this booking''s tickets were returned to the event and tier '
    'counters. NULL means the booking still occupies its seats. Guards against '
    'double-release, which would oversell the event.';

-- Partial index: the release path checks this column per booking, and released
-- bookings are a minority of rows.
CREATE INDEX IF NOT EXISTS idx_bookings_seats_not_released
    ON bookings (event_id)
    WHERE seats_released_at IS NULL;

-- ── Backfill ─────────────────────────────────────────────────────────────────
--
-- Every booking already in a terminal state is marked released, whether or not
-- it truly was. We cannot tell from the data which cancelled rows the expiry
-- worker decremented and which refunded rows were never decremented at all — so
-- rather than guess, we mark them all released and then recompute tickets_sold
-- from scratch below. That makes the counters correct and guarantees no
-- historical row can be released a second time by the new code.
UPDATE bookings
SET seats_released_at = COALESCE(refunded_at, updated_at, created_at)
WHERE seats_released_at IS NULL
  AND booking_status IN ('cancelled', 'refunded', 'expired');

-- ── One-time correction ──────────────────────────────────────────────────────
--
-- Recompute every event's counter from the bookings that actually hold seats.
-- This clears the drift accumulated by every refund since launch.
UPDATE events e
SET tickets_sold = COALESCE((
        SELECT SUM(b.ticket_quantity)
        FROM bookings b
        WHERE b.event_id = e.id
          AND b.booking_status IN ('confirmed', 'pending')
    ), 0),
    updated_at = CURRENT_TIMESTAMP
WHERE e.tickets_sold IS DISTINCT FROM COALESCE((
        SELECT SUM(b.ticket_quantity)
        FROM bookings b
        WHERE b.event_id = e.id
          AND b.booking_status IN ('confirmed', 'pending')
    ), 0);

-- Same for ticket tiers, which carry their own counter.
UPDATE ticket_types t
SET tickets_sold = COALESCE((
        SELECT SUM(b.ticket_quantity)
        FROM bookings b
        WHERE b.ticket_type_id = t.id
          AND b.booking_status IN ('confirmed', 'pending')
    ), 0),
    updated_at = CURRENT_TIMESTAMP
WHERE t.tickets_sold IS DISTINCT FROM COALESCE((
        SELECT SUM(b.ticket_quantity)
        FROM bookings b
        WHERE b.ticket_type_id = t.id
          AND b.booking_status IN ('confirmed', 'pending')
    ), 0);