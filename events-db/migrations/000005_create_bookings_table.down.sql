-- migrations/000005_create_bookings_table.down.sql

DROP TRIGGER IF EXISTS update_bookings_updated_at ON bookings;
DROP TRIGGER IF EXISTS trigger_set_booking_number ON bookings;
DROP FUNCTION IF EXISTS set_booking_number();
DROP FUNCTION IF EXISTS generate_booking_number();
DROP TABLE IF EXISTS bookings CASCADE;