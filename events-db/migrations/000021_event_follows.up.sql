-- migrations/000021_event_follows.up.sql
--
-- Event follow ("save / like") system.
--
-- A user follows an event to:
--   1. Have it appear on their /user/following feed.
--   2. Receive reminder notifications before the event starts.
--
-- Reminder flags prevent duplicate notifications across server restarts.
-- The reminder worker checks both flags every hour and flips them to TRUE
-- once a notification has been sent, so they are idempotent.

CREATE TABLE event_follows (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID        NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    event_id     UUID        NOT NULL REFERENCES events(id) ON DELETE CASCADE,

    -- Reminder sent flags — set to TRUE once the notification fires so it
    -- is never sent twice even if the worker runs multiple times in the window.
    reminded_7_days BOOLEAN NOT NULL DEFAULT FALSE,
    reminded_1_day  BOOLEAN NOT NULL DEFAULT FALSE,

    followed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_event_follows UNIQUE (user_id, event_id)
);

-- Primary lookup: "what events does user X follow?" (used by the feed).
CREATE INDEX idx_event_follows_user    ON event_follows(user_id, followed_at DESC);

-- Used by the reminder worker: "which follows need a 7-day or 1-day reminder?"
CREATE INDEX idx_event_follows_reminders
    ON event_follows(event_id)
    WHERE reminded_7_days = FALSE OR reminded_1_day = FALSE;