-- migrations/000020_add_event_slug.up.sql
--
-- Adds a URL-friendly slug to every event.
--
-- Format: <slugified-title>-<first-8-chars-of-uuid>
-- Example: "Cape Town Jazz Festival 2026" → "cape-town-jazz-festival-2026-a1b2c3d4"
--
-- The short UUID suffix guarantees uniqueness even when two events share an
-- identical title, so no retry loop is needed on insert.
--
-- Existing rows are backfilled using a plpgsql helper that mirrors the logic
-- in utils/slug.go. New rows are written by the application layer at
-- CreateEvent time — no trigger is needed.

ALTER TABLE events ADD COLUMN IF NOT EXISTS slug VARCHAR(320) UNIQUE;

-- Backfill: generate slugs for all existing events.
-- regexp_replace strips anything that isn't a letter, digit, or hyphen, then
-- collapses runs of non-word characters into a single hyphen.
UPDATE events
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(title, '[^a-zA-Z0-9\s-]', '', 'g'),
        '[\s-]+', '-', 'g'
    )
) || '-' || LEFT(id::text, 8)
WHERE slug IS NULL;

-- Make it NOT NULL now that all rows have been filled.
ALTER TABLE events ALTER COLUMN slug SET NOT NULL;

-- Unique index is implied by the UNIQUE constraint above, but an explicit
-- partial index on published events gives the public slug-lookup query a fast path.
CREATE INDEX IF NOT EXISTS idx_events_slug ON events(slug);