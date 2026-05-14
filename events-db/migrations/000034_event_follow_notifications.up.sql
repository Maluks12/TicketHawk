-- migrations/000034_event_follow_notifications.up.sql
--
-- Tracks whether sell-out threshold notifications have already been dispatched
-- for an event so the selling-out worker never sends the same alert twice,
-- even across restarts.
--
-- Two thresholds:
--   80 % capacity sold  → "Selling fast" push to all followers
--   100% capacity sold  → "Sold out" push to all followers
--
-- These are event-level flags (not per-follow) because the notification
-- fires once per event, not once per follower.  The worker fans out to all
-- followers in one sweep after flipping the flag.

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS notified_80pct    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS notified_sold_out BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index used by the selling-out worker.
-- Only events that still need at least one notification show up here.
CREATE INDEX IF NOT EXISTS idx_events_sell_out_pending
    ON events (id, tickets_sold, max_capacity)
    WHERE (notified_80pct = FALSE OR notified_sold_out = FALSE)
      AND status = 'published'
      AND max_capacity IS NOT NULL;