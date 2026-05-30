-- 000042_rotating_qr.down.sql

DROP INDEX IF EXISTS idx_ta_scanned_at;

ALTER TABLE ticket_assignments
    DROP COLUMN IF EXISTS qr_scanned_at,
    DROP COLUMN IF EXISTS scanned_by_organizer_id;