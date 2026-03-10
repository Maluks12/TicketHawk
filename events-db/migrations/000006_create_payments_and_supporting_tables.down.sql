-- migrations/000006_create_payments_and_supporting_tables.down.sql

DROP TABLE IF EXISTS attendee_demographics;
DROP FUNCTION IF EXISTS refresh_event_revenue_summary();
DROP MATERIALIZED VIEW IF EXISTS event_revenue_summary;
DROP TABLE IF EXISTS event_views;
DROP TRIGGER IF EXISTS update_waitlist_updated_at ON waitlist;
DROP TABLE IF EXISTS waitlist CASCADE;
DROP TRIGGER IF EXISTS update_payments_updated_at ON payments;
DROP TABLE IF EXISTS payments CASCADE;