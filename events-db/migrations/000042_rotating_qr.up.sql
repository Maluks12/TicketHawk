-- 000042_rotating_qr.up.sql
--
-- Adds two columns to ticket_assignments to support rotating QR verification:
--
--   qr_scanned_at            — set when the assignment is scanned at the door.
--                              NULL means not yet used. A NOT NULL value means
--                              the ticket was already admitted — reject re-entry.
--
--   scanned_by_organizer_id  — records which organizer account performed the scan.
--                              Useful for audit trails and dispute resolution.
--
-- These are nullable so existing rows are unaffected and no backfill is needed.

ALTER TABLE ticket_assignments
    ADD COLUMN IF NOT EXISTS qr_scanned_at           TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS scanned_by_organizer_id UUID REFERENCES organizers(id);

-- Partial index: only index scanned tickets, keeping the index tiny.
-- Used to quickly find "already admitted" tickets without a full table scan.
CREATE INDEX IF NOT EXISTS idx_ta_scanned_at
    ON ticket_assignments (qr_scanned_at)
    WHERE qr_scanned_at IS NOT NULL;