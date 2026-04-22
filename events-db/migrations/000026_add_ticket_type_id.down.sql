-- migrations/000026_add_ticket_type_id.down.sql
ALTER TABLE bookings          DROP COLUMN IF EXISTS ticket_type_name;
ALTER TABLE bookings          DROP COLUMN IF EXISTS ticket_type_id;
ALTER TABLE ticket_reservations DROP COLUMN IF EXISTS ticket_type_id;
ALTER TABLE cart_items        DROP COLUMN IF EXISTS ticket_type_id;