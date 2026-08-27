-- ============================================================================
-- 000078: Follow an organiser.
--
-- WHY A SEPARATE TABLE AND NOT A COLUMN ON event_follows
--
-- Following an organiser and following an event are different promises.
-- Following an event means "remind me before this happens"; following an
-- organiser means "tell me when they announce something new". The reminder
-- worker's reminded_7_days / reminded_1_day flags are meaningless for an
-- organiser, who has no start time, and the fan-out this table drives is
-- keyed on publication rather than on a date approaching.
--
-- WHY followers_notified_at LIVES ON events
--
-- The fan-out has to fire exactly once per event, and there are THREE ways an
-- event reaches 'published':
--
--   AdminApproveEvent          — the review queue
--   ApproveEventWithFeature    — approve + feature in one action
--   AdminPromoteEvent          — approve + promote in one click, which
--                                publishes as a side effect of promoting
--
-- Hooking the notification into any one of them would miss the other two, and
-- hooking all three means three places to keep in step — plus whatever the
-- fourth path turns out to be. A latch on the row lets a worker ask the only
-- question that actually matters ("is this event live and unannounced?")
-- without caring which code path got it there. It also survives a takedown and
-- re-approval without re-notifying, which a hook would not.
--
-- WHY THERE IS NO DENORMALISED follower_count
--
-- COUNT(*) over idx_organizer_follows_organizer is an index-only scan. At the
-- scale where that stops being fast — low hundreds of thousands of follows per
-- organiser — a counter column would need its own trigger and its own drift
-- reconciliation, and would be wrong in a way nobody notices for months. Add
-- it when the query shows up in pg_stat_statements, not before.
-- ============================================================================

CREATE TABLE IF NOT EXISTS organizer_follows (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID        NOT NULL REFERENCES users(id)      ON DELETE CASCADE,
    organizer_id UUID        NOT NULL REFERENCES organizers(id) ON DELETE CASCADE,
    followed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_organizer_follows UNIQUE (user_id, organizer_id)
);

-- "Which organisers does this user follow?" — the Following tab.
CREATE INDEX IF NOT EXISTS idx_organizer_follows_user
    ON organizer_follows(user_id, followed_at DESC);

-- "Who follows this organiser?" — the follower count and the fan-out.
-- Also what makes the count an index-only scan.
CREATE INDEX IF NOT EXISTS idx_organizer_follows_organizer
    ON organizer_follows(organizer_id);

-- ── New-event announcement latch ────────────────────────────────────────────

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS followers_notified_at TIMESTAMPTZ;

-- Backfill: every event that is ALREADY published is marked as announced.
--
-- Without this, the first run of the worker after deploy would treat the entire
-- back catalogue as newly published and fan out a notification for every live
-- event to every follower — on a table that has no rows yet, so the immediate
-- blast radius is zero, but the moment the first person follows an organiser
-- they would receive an announcement for each of that organiser's existing
-- events. Marking history as already-announced is the only correct starting
-- state.
UPDATE events
SET    followers_notified_at = COALESCE(published_at, created_at, NOW())
WHERE  followers_notified_at IS NULL
  AND  status IN ('published', 'published_private', 'completed', 'cancelled');

-- Partial index covering exactly the worker's predicate. The worker polls every
-- minute forever, so this wants to be an index probe over a handful of rows
-- rather than a scan of the events table.
--
-- published_private is excluded from the index, not just the query: a link-only
-- event is deliberately undiscoverable, and announcing it to an organiser's
-- followers would defeat the entire point of choosing that visibility.
CREATE INDEX IF NOT EXISTS idx_events_pending_announcement
    ON events(published_at)
    WHERE followers_notified_at IS NULL
      AND status = 'published'
      AND visibility = 'public';