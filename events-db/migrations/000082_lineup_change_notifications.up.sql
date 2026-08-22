-- ============================================================================
-- 000082: Line-up change notifications.
--
-- THE GAP THIS CLOSES
--
-- Phase 4 made line-up changes apply IMMEDIATELY on a live event, with no
-- admin review, and that was the right call: an act drops out three days before
-- a festival and the replacement is announced the same afternoon, so a line-up
-- sitting in a review queue is wrong on exactly the days most people are
-- looking at it.
--
-- But it left a real hole. Someone who bought a ticket to see a headliner who
-- has since been removed currently finds out by opening the page — or by
-- arriving. The organiser can make that change silently.
--
-- The fix is NOT to block the change. A correct page nobody was told about is
-- still better than a stale one; blocking would give us a stale page AND an
-- uninformed buyer. The fix is to tell the people who have a stake in it.
--
-- WHO GETS TOLD, AND WHY IT IS TWO DIFFERENT GROUPS
--
--   ticket holders   have paid money against a specific line-up. They are the
--                    reason this exists, and they are notified for ANY change.
--
--   organiser        followers asked to hear about this organiser's events.
--   followers        A line-up change is news; a corrected typo in a stage name
--                    is not. They are notified only when an act is REMOVED or
--                    ADDED, never for edits to an existing entry.
--
-- WHY A SNAPSHOT COLUMN AND NOT A DIFF AT WRITE TIME
--
-- The handler could compare old and new in memory and fire notifications
-- directly. That fails in three ways: it puts a fan-out on the organiser's
-- save request, it re-notifies on every save even when nothing material moved,
-- and it misses changes made through the admin edit-request path, which writes
-- the same table by a different route.
--
-- Storing a SNAPSHOT of what was last announced lets a worker answer "has the
-- line-up materially changed since we last told anyone?" — which covers every
-- write path, present and future, and is idempotent under retry. Same shape as
-- followers_notified_at in 000078, for the same reasons.
-- ============================================================================

ALTER TABLE events
    -- Sorted, normalised list of the line-up names last announced, as JSONB:
    --   {"performer": ["a","b"], "team": ["x","y"]}
    --
    -- Names only. Stage, set time, image and link changes are real edits but
    -- they are not "the act you paid to see is gone", and notifying on them
    -- would train people to ignore the notification that matters.
    ADD COLUMN IF NOT EXISTS lineup_announced_snapshot JSONB,

    -- When the last line-up change notification went out. Rate-limits the
    -- worker: an organiser building a line-up entry by entry would otherwise
    -- fire a notification per save.
    ADD COLUMN IF NOT EXISTS lineup_notified_at TIMESTAMPTZ;

-- Backfill: every currently-live event is seeded with its CURRENT line-up as
-- the already-announced state.
--
-- Without this, the worker's first run would treat every existing line-up as
-- newly changed and notify every ticket holder on the platform that an event
-- they booked has had its line-up updated — when nothing has changed at all.
-- Migration 000078 needed exactly the same guard.
UPDATE events e
SET    lineup_announced_snapshot = COALESCE(
           (
             SELECT jsonb_object_agg(kind, names)
             FROM (
               SELECT l.kind, jsonb_agg(l.name ORDER BY l.name) AS names
               FROM   event_lineup l
               WHERE  l.event_id = e.id
               GROUP  BY l.kind
             ) grouped
           ),
           '{}'::jsonb
       ),
       lineup_notified_at = NOW()
WHERE  e.lineup_announced_snapshot IS NULL
  AND  e.status IN ('published', 'published_private', 'completed', 'cancelled');

-- Covers the worker's predicate. It polls forever, so this wants to be an
-- index probe over a handful of rows rather than a scan.
--
-- Only events that are LIVE and still upcoming are candidates: a line-up
-- change on a finished event is a records correction, and nobody needs a push
-- about an act that did not play last month.
CREATE INDEX IF NOT EXISTS idx_events_lineup_watch
    ON events(id)
    WHERE status IN ('published', 'published_private');