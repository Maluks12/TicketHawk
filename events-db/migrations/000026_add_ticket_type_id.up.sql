-- migrations/000026_add_ticket_type_id.up.sql
--
-- Threads the ticket_type_id FK through every table that participates in the
-- booking flow: cart → reservation → booking.
-- All columns are nullable so existing rows and flat-priced events keep working.

-- cart_items: which ticket tier the buyer chose
ALTER TABLE cart_items
    ADD COLUMN ticket_type_id UUID REFERENCES ticket_types(id) ON DELETE SET NULL;

CREATE INDEX idx_cart_items_ticket_type_id
    ON cart_items(ticket_type_id) WHERE ticket_type_id IS NOT NULL;

-- ticket_reservations: preserve the tier through the 10-minute hold window
ALTER TABLE ticket_reservations
    ADD COLUMN ticket_type_id UUID REFERENCES ticket_types(id) ON DELETE SET NULL;

CREATE INDEX idx_reservations_ticket_type_id
    ON ticket_reservations(ticket_type_id) WHERE ticket_type_id IS NOT NULL;

-- bookings: record which tier was booked and snapshot its name for receipts /
--           emails even if the organiser later renames or deletes the type
ALTER TABLE bookings
    ADD COLUMN ticket_type_id   UUID REFERENCES ticket_types(id) ON DELETE SET NULL,
    ADD COLUMN ticket_type_name VARCHAR(100);

CREATE INDEX idx_bookings_ticket_type_id
    ON bookings(ticket_type_id) WHERE ticket_type_id IS NOT NULL;