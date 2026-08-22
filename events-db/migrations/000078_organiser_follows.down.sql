-- ============================================================================
-- 000078 down.
--
-- Dropping organizer_follows destroys every follow relationship. There is no
-- way to reconstruct it — unlike the slug in 000077, which can at least be
-- regenerated from business_name. Re-applying the up migration leaves every
-- organiser with zero followers.
--
-- followers_notified_at is dropped with it. If this migration is later
-- re-applied, the backfill in the up file re-marks all existing published
-- events as already-announced, so a round trip does NOT cause a notification
-- blast. That is deliberate: the destructive failure mode here is losing
-- follows, not spamming people.
-- ============================================================================

DROP INDEX IF EXISTS idx_events_pending_announcement;

ALTER TABLE events
    DROP COLUMN IF EXISTS followers_notified_at;

DROP TABLE IF EXISTS organizer_follows;