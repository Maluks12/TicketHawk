-- Reverse migration 000037
DROP INDEX IF EXISTS idx_merchandise_pending_review;
DROP INDEX IF EXISTS idx_events_pending_review;

ALTER TABLE merchandise
    DROP COLUMN IF EXISTS rejection_reason,
    DROP COLUMN IF EXISTS approval_status;

ALTER TABLE events
    DROP COLUMN IF EXISTS rejection_reason;