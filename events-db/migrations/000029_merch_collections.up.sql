-- migrations/000029_merch_collections.up.sql

-- Merch collections — organiser/admin-curated groups of merchandise items.
-- A collection is distinct from a category: category drives colour theming,
-- collection drives UI grouping in the carousel/modal.
CREATE TABLE merch_collections (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- organizer_id is the user_id from the JWT (matches c.GetString("user_id")).
    -- NULL means the collection was created by an admin, not an organiser.
    organizer_id  UUID,
    name          VARCHAR(255) NOT NULL,
    description   TEXT,
    display_order INTEGER      NOT NULL DEFAULT 0,
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_merch_collections_organizer ON merch_collections(organizer_id)
    WHERE organizer_id IS NOT NULL;
CREATE INDEX idx_merch_collections_active ON merch_collections(is_active);

-- Add collection_id FK to merchandise.
-- ON DELETE SET NULL — deleting a collection un-assigns its items rather than
-- cascading a delete, keeping order history intact.
ALTER TABLE merchandise
    ADD COLUMN collection_id UUID REFERENCES merch_collections(id) ON DELETE SET NULL;

CREATE INDEX idx_merch_collection_id ON merchandise(collection_id)
    WHERE collection_id IS NOT NULL;