-- ============================================================
-- Migration: Trigram indexes for fuzzy event search
-- File: migrations/000040_trgm_search_indexes.up.sql
--
-- pg_trgm is already enabled (migration 000001).
-- These GIN indexes allow word_similarity() queries on title,
-- venue_name and city to use index scans instead of seq scans.
-- ============================================================

CREATE INDEX idx_events_title_trgm    ON events USING GIN (title       gin_trgm_ops);
CREATE INDEX idx_events_venue_trgm    ON events USING GIN (venue_name  gin_trgm_ops);
CREATE INDEX idx_events_city_trgm     ON events USING GIN (city        gin_trgm_ops);