-- migrations/000048_admin_promoted_events.down.sql

DROP INDEX IF EXISTS idx_events_admin_promoted;

ALTER TABLE events
    DROP COLUMN IF EXISTS admin_promoted_at,
    DROP COLUMN IF EXISTS admin_promoted;