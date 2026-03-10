-- migrations/000004_create_events_table.down.sql

DROP TRIGGER IF EXISTS update_events_updated_at ON events;
DROP TRIGGER IF EXISTS events_search_vector_update ON events;
DROP FUNCTION IF EXISTS events_search_trigger();
DROP TABLE IF EXISTS events CASCADE;