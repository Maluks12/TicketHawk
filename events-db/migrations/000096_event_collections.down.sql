DROP INDEX IF EXISTS idx_events_event_collection;
ALTER TABLE events DROP COLUMN IF EXISTS event_collection_id;
DROP INDEX IF EXISTS idx_event_collections_organizer;
DROP TABLE IF EXISTS event_collections;