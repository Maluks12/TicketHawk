-- ============================================================
-- Migration 000037: Apply event and merch approval columns
--
-- Depends on 000036 (enum values must exist before use).
-- Runs in its own transaction so the new enum values are
-- committed and safe to reference.
-- ============================================================

-- 1. Add rejection_reason to events
ALTER TABLE events
    ADD COLUMN IF NOT EXISTS rejection_reason TEXT;

-- 2. Add approval columns to merchandise
ALTER TABLE merchandise
    ADD COLUMN IF NOT EXISTS approval_status  merch_approval_status NOT NULL DEFAULT 'pending_review',
    ADD COLUMN IF NOT EXISTS rejection_reason TEXT;

-- 3. Mark all currently active merch as already approved (pre-existing items)
UPDATE merchandise SET approval_status = 'approved' WHERE is_active = TRUE;

-- 4. Indexes for admin queue pages
CREATE INDEX IF NOT EXISTS idx_events_pending_review
    ON events(status, created_at)
    WHERE status = 'pending_review';

CREATE INDEX IF NOT EXISTS idx_merchandise_pending_review
    ON merchandise(approval_status, created_at)
    WHERE approval_status = 'pending_review';