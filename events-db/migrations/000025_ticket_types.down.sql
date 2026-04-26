-- migrations/000025_ticket_types.down.sql
DROP TRIGGER IF EXISTS trg_sync_event_price ON ticket_types;
DROP FUNCTION IF EXISTS sync_event_price_from_ticket_types();
DROP TRIGGER IF EXISTS update_ticket_types_updated_at ON ticket_types;
DROP TABLE IF EXISTS ticket_types;