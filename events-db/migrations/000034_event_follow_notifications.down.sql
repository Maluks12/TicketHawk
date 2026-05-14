-- migrations/000034_event_follow_notifications.down.sql

DROP INDEX IF EXISTS idx_events_sell_out_pending;

ALTER TABLE events
    DROP COLUMN IF EXISTS notified_80pct,
    DROP COLUMN IF EXISTS notified_sold_out;