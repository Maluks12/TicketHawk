-- Allow bookings to exist without a linked event so that merch-only carts can
-- complete the payment flow. Ticket and mixed carts still produce bookings with
-- a non-NULL event_id; this change only widens the constraint.
ALTER TABLE bookings ALTER COLUMN event_id DROP NOT NULL;

-- session_id for guest merch-only bookings (mirrors the carts / merch_orders pattern).
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS session_id UUID;

-- booking_type distinguishes regular ticket bookings from merch-only ones.
-- Existing rows default to 'ticket'.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_type WHERE typname = 'booking_type'
    ) THEN
        CREATE TYPE booking_type AS ENUM ('ticket', 'merch_only');
    END IF;
END $$;

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS booking_type booking_type NOT NULL DEFAULT 'ticket';

CREATE INDEX IF NOT EXISTS idx_bookings_session_id
    ON bookings(session_id) WHERE session_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_bookings_booking_type
    ON bookings(booking_type);