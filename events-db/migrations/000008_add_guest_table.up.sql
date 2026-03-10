-- Migration: Add guest booking support
-- Run this against your database before deploying the updated code.

-- 1. Make user_id nullable — guest bookings have no account
ALTER TABLE bookings
    ALTER COLUMN user_id DROP NOT NULL;

-- 2. Add is_guest flag so queries can efficiently filter guest vs auth bookings
ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS is_guest BOOLEAN NOT NULL DEFAULT FALSE;

-- 3. Index for guest lookup: booking_number + email (case-insensitive)
CREATE INDEX IF NOT EXISTS idx_bookings_guest_lookup
    ON bookings (booking_number, lower(attendee_email))
    WHERE is_guest = TRUE;

-- 4. Backfill: any existing bookings with a user_id are not guests
UPDATE bookings SET is_guest = FALSE WHERE user_id IS NOT NULL;

-- 5. Constraint: authenticated bookings must have user_id,
--    guest bookings must have attendee_email.
ALTER TABLE bookings
    ADD CONSTRAINT chk_booking_identity CHECK (
        (is_guest = FALSE AND user_id IS NOT NULL)
        OR
        (is_guest = TRUE AND attendee_email IS NOT NULL)
    );