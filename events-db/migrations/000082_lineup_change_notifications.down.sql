-- ============================================================================
-- 000082 down.
--
-- Safe. Nothing but bookkeeping is lost: the columns record what was last
-- ANNOUNCED, not any part of the line-up itself, which lives in event_lineup
-- and is untouched here.
--
-- Re-applying the up migration re-seeds every live event with its current
-- line-up as already-announced, so a round trip does NOT cause a notification
-- blast. Same property as 000078.
-- ============================================================================

DROP INDEX IF EXISTS idx_events_lineup_watch;

ALTER TABLE events
    DROP COLUMN IF EXISTS lineup_notified_at,
    DROP COLUMN IF EXISTS lineup_announced_snapshot;