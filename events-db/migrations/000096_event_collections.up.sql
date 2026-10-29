-- ============================================================================
-- 000096: Event collections.
--
-- WHAT THIS ADDS
--
-- An organiser running several dates of one thing — a pop-up tour across
-- East London, Port Elizabeth, Johannesburg and Cape Town, say — can group
-- those events into a COLLECTION with its own title, description and cover.
--
-- The home feed then shows ONE card for the collection instead of one card per
-- date, so a twelve-stop tour does not push everyone else's events off the
-- first screen. Tapping the card opens the collection page listing every date.
-- Search, category pages and the map still list each event individually, so a
-- visitor searching "Cape Town" still finds the Castle of Good Hope date.
--
-- NOT TO BE CONFUSED WITH merch_collections (migration 000029), which group
-- merchandise and own the /v1/collections routes. These are event_collections
-- and live under /v1/event-collections.
--
-- ONE COLLECTION PER EVENT
--
-- Membership is a nullable column on events rather than a join table. An event
-- in two collections would appear twice in the feed under two different cards,
-- which is exactly the clutter this exists to remove; the column makes that
-- impossible rather than merely discouraged.
--
-- NO REVIEW STEP
--
-- A collection never makes anything visible on its own: it only ever shows
-- events that are already published and public, which have been through their
-- own review. Its title, description and cover are presentation, the same as
-- an organiser's public page.
-- ============================================================================

CREATE TABLE IF NOT EXISTS event_collections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- A USER id, the same convention as events.organizer_id (which also
    -- references users(id) despite its name). Keeping the two identical is
    -- what lets membership be checked with a plain equality between them.
    organizer_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- title + first 8 characters of the id (utils.GenerateSlug), so it is
    -- unique without a retry loop. Fixed at creation: shared links keep
    -- working after a rename, the same rule as event and organiser slugs.
    slug            TEXT NOT NULL UNIQUE,

    title           VARCHAR(120) NOT NULL,
    description     TEXT,
    cover_image_url TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_event_collections_title CHECK (char_length(btrim(title)) >= 2)
);

CREATE INDEX IF NOT EXISTS idx_event_collections_organizer
    ON event_collections (organizer_id);

-- Deleting a collection releases its events back to the feed as individual
-- cards; it never touches the events themselves.
ALTER TABLE events
    ADD COLUMN IF NOT EXISTS event_collection_id UUID
        REFERENCES event_collections(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_events_event_collection
    ON events (event_collection_id)
    WHERE event_collection_id IS NOT NULL;