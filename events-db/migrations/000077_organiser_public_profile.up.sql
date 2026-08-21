-- ============================================================================
-- 000077: Organiser public profile — slug, cover art, socials, visibility.
--
-- WHY A SLUG AND NOT THE UUID
--
-- The organiser page is a public, indexable marketing surface, so the URL is
-- part of the product: /organiser/wilderness-beach reads as a brand, and
-- /organiser/3f9a1c02-… does not. Events already accept either form on their
-- detail route, and the organiser route will do the same, so nothing breaks if
-- a client only has the id to hand.
--
-- WHY THE SLUG IS ASSIGNED BY A TRIGGER
--
-- Generating it in Go means a read-then-write across two statements, which
-- races: two organisers registering the same business name in the same second
-- both see the name as free and one insert then violates the unique index.
-- A BEFORE INSERT trigger resolves the collision inside the same statement
-- that takes the row lock, so there is no window. It also means every write
-- path gets a slug for free — CreateOrganizer, the admin invite flow, seed
-- scripts, and any future importer — rather than each remembering to call a
-- helper.
--
-- WHY THE SLUG NEVER CHANGES ON RENAME
--
-- The trigger is INSERT-only. A renamed organiser keeps their original slug,
-- which is the cheapest way to guarantee that a link shared on WhatsApp in
-- March still resolves in October. Changing it would need a redirect table to
-- stay honest, and that is not worth building for an event that happens rarely
-- and has no user-visible upside.
--
-- WHY SOCIALS ARE A TABLE AND NOT COLUMNS
--
-- Fixed columns (instagram_url, tiktok_url, …) mean a migration every time a
-- platform matters — and platforms turn over faster than schemas should. A row
-- per link also gives the organiser an explicit display order, which columns
-- cannot express at all.
--
-- The host allow-list that stops an "Instagram" link pointing at a payment
-- site lives in Go (validateOrganizerLink), not in a CHECK, because it needs
-- to produce an error message a human can act on and it will grow. The CHECK
-- here is a backstop for the one rule that must hold no matter what writes the
-- row: the URL is https.
-- ============================================================================

-- ── Profile columns ─────────────────────────────────────────────────────────

ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS slug                   VARCHAR(120),
    -- Wide hero image for the organiser page. Distinct from logo_url, which is
    -- the small square mark used in event cards and the detail sidebar.
    ADD COLUMN IF NOT EXISTS cover_image_url        TEXT,
    -- Organiser-controlled kill switch. Defaults true so existing organisers
    -- are opted in, which is the behaviour they would expect from a feature
    -- that simply lists events they have already published publicly.
    ADD COLUMN IF NOT EXISTS public_profile_enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- ── Slug assignment ─────────────────────────────────────────────────────────
--
-- Defined before the backfill so the same collision logic serves both. The
-- loop widens the UUID suffix rather than counting (-2, -3, …): a counter has
-- to re-probe the table on every attempt against rows that may not be visible
-- to this transaction, whereas the UUID is already unique and only needs
-- enough characters to disambiguate.

CREATE OR REPLACE FUNCTION organizers_set_slug() RETURNS trigger AS $$
DECLARE
    base      TEXT;
    candidate TEXT;
    n         INT := 0;
BEGIN
    -- An explicitly supplied slug wins. Lets an admin set a vanity slug and
    -- lets the backfill below reuse this function without recursion trouble.
    IF NEW.slug IS NOT NULL AND NEW.slug <> '' THEN
        RETURN NEW;
    END IF;

    base := NULLIF(
        trim(BOTH '-' FROM regexp_replace(lower(coalesce(NEW.business_name, '')), '[^a-z0-9]+', '-', 'g')),
        ''
    );

    -- A business name of only punctuation or non-Latin script collapses to
    -- empty. Fall back rather than produce a bare UUID slug.
    IF base IS NULL THEN
        base := 'organiser';
    END IF;

    base      := left(base, 100);
    candidate := base;

    WHILE EXISTS (SELECT 1 FROM organizers WHERE slug = candidate) LOOP
        n := n + 1;
        candidate := base || '-' || substr(replace(NEW.id::text, '-', ''), 1, 3 + n);
        -- Past this point the UUID prefix is long enough that a collision
        -- means something is badly wrong; use the whole thing and stop.
        IF n > 8 THEN
            candidate := base || '-' || replace(NEW.id::text, '-', '');
            EXIT;
        END IF;
    END LOOP;

    NEW.slug := candidate;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;

-- BEFORE INSERT only. See the rename note in the header.
--
-- NEW.id is populated here even though it comes from a column DEFAULT:
-- Postgres applies defaults before BEFORE-row triggers fire, so the suffix
-- above always has a real UUID to draw on.
DROP TRIGGER IF EXISTS organizers_slug_insert ON organizers;
CREATE TRIGGER organizers_slug_insert
    BEFORE INSERT ON organizers
    FOR EACH ROW
    EXECUTE FUNCTION organizers_set_slug();

-- ── Backfill ────────────────────────────────────────────────────────────────
--
-- Set-based rather than a row loop calling the trigger function: the trigger's
-- EXISTS check reads a table that is empty of slugs at this point, so every
-- row would see itself as unique and duplicates would survive to the index
-- creation below. ROW_NUMBER over the base slug resolves them in one pass.

WITH base AS (
    SELECT
        id,
        COALESCE(
            NULLIF(trim(BOTH '-' FROM regexp_replace(lower(coalesce(business_name, '')), '[^a-z0-9]+', '-', 'g')), ''),
            'organiser'
        ) AS b
    FROM organizers
    WHERE slug IS NULL
),
numbered AS (
    SELECT
        id,
        left(b, 100) AS b,
        ROW_NUMBER() OVER (PARTITION BY left(b, 100) ORDER BY id) AS rn
    FROM base
)
UPDATE organizers o
SET slug = CASE
        WHEN n.rn = 1 THEN n.b
        ELSE n.b || '-' || substr(replace(o.id::text, '-', ''), 1, 4)
    END
FROM numbered n
WHERE n.id = o.id;

-- Constraints applied only after every row has a value.
ALTER TABLE organizers ALTER COLUMN slug SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organizers_slug ON organizers(slug);

-- Covers the public page's own lookup: slug plus the visibility predicates the
-- query always carries, so the gate costs an index probe rather than a heap
-- fetch on rows the caller may not be allowed to see.
CREATE INDEX IF NOT EXISTS idx_organizers_public_lookup
    ON organizers(slug)
    WHERE public_profile_enabled = TRUE AND kyc_level >= 2;

-- ── Social and website links ────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS organizer_links (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organizer_id UUID NOT NULL REFERENCES organizers(id) ON DELETE CASCADE,
    platform     VARCHAR(32) NOT NULL,
    url          TEXT        NOT NULL,
    sort_order   SMALLINT    NOT NULL DEFAULT 0,
    created_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- One row per platform per organiser. Two Instagram links is never a
    -- deliberate choice, and allowing it would make the display order
    -- ambiguous.
    CONSTRAINT uq_organizer_link_platform UNIQUE (organizer_id, platform),

    CONSTRAINT chk_organizer_link_platform CHECK (platform IN (
        'instagram', 'x', 'facebook', 'tiktok',
        'youtube', 'linkedin', 'whatsapp', 'spotify', 'website'
    )),

    -- Backstop only — see the header note. http:// is refused outright rather
    -- than upgraded, because silently rewriting a URL an organiser typed is a
    -- worse failure than telling them it was rejected.
    CONSTRAINT chk_organizer_link_https CHECK (url ~* '^https://'),

    CONSTRAINT chk_organizer_link_url_len CHECK (char_length(url) BETWEEN 12 AND 500)
);

CREATE INDEX IF NOT EXISTS idx_organizer_links_organizer
    ON organizer_links(organizer_id, sort_order);

DROP TRIGGER IF EXISTS update_organizer_links_updated_at ON organizer_links;
CREATE TRIGGER update_organizer_links_updated_at
    BEFORE UPDATE ON organizer_links
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();