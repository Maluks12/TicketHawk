-- Revert: delete merch-only bookings (no event_id) then re-add NOT NULL.
DELETE FROM bookings WHERE event_id IS NULL;
ALTER TABLE bookings ALTER COLUMN event_id SET NOT NULL;

DROP INDEX IF EXISTS idx_bookings_session_id;
DROP INDEX IF EXISTS idx_bookings_booking_type;
ALTER TABLE bookings DROP COLUMN IF EXISTS session_id;
ALTER TABLE bookings DROP COLUMN IF EXISTS booking_type;
DROP TYPE IF EXISTS booking_type;