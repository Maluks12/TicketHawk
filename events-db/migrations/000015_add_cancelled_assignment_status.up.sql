-- migrations/000015_add_cancelled_assignment_status.up.sql
--
-- Adds 'cancelled' to the assignment_status enum so RequestRefundWithCascade
-- can mark slots as cancelled rather than repurposing 'revoked'.
-- Also adds an index to speed up the booking_status join in QR verification.

ALTER TYPE assignment_status ADD VALUE IF NOT EXISTS 'cancelled';

-- Index to make the booking-status guard in VerifyAssignmentQRCode fast.
-- The join goes: ticket_assignments → bookings → events, so an index on
-- bookings(id, booking_status) helps the planner avoid a full scan.
CREATE INDEX IF NOT EXISTS idx_bookings_id_status
    ON bookings(id, booking_status);