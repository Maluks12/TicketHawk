-- migrations/000020_add_event_slug.down.sql
DROP INDEX IF EXISTS idx_events_slug;
ALTER TABLE events DROP COLUMN IF EXISTS slug;