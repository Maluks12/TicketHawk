-- ============================================================================
-- 000077 down.
--
-- Dropping `slug` loses every organiser's public URL. Any link already shared
-- will 404 after a re-migrate, because the trigger regenerates slugs from
-- business_name and an organiser who has since renamed gets a different one.
-- That is acceptable for a rollback but is the reason to avoid round-tripping
-- this migration casually in production.
-- ============================================================================

DROP TRIGGER IF EXISTS update_organizer_links_updated_at ON organizer_links;
DROP TABLE IF EXISTS organizer_links;

DROP TRIGGER IF EXISTS organizers_slug_insert ON organizers;
DROP FUNCTION IF EXISTS organizers_set_slug();

DROP INDEX IF EXISTS idx_organizers_public_lookup;
DROP INDEX IF EXISTS idx_organizers_slug;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS public_profile_enabled,
    DROP COLUMN IF EXISTS cover_image_url,
    DROP COLUMN IF EXISTS slug;