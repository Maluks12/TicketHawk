-- ============================================================================
-- 000080 down.
--
-- Dropping event_info_items destroys every piece of information organisers have
-- entered — parking arrangements, accessibility notes, age restrictions. None
-- of it is reconstructible, and some of it (age restriction in particular) is
-- material to tickets already sold.
--
-- Order matters: items references catalogue with ON DELETE RESTRICT, so the
-- items table goes first.
-- ============================================================================

DROP TRIGGER IF EXISTS update_event_info_items_updated_at ON event_info_items;
DROP INDEX IF EXISTS idx_event_info_items_event;
DROP TABLE IF EXISTS event_info_items;
DROP TABLE IF EXISTS event_info_catalogue;