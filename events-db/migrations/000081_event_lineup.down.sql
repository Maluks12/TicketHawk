-- ============================================================================
-- 000081 down.
--
-- Safe in a way most of these are not: events.performers and events.teams were
-- never dropped, and ReplaceEventLineup dual-writes the names back to them on
-- every save. So rolling back loses the IMAGES, stages, set times and links,
-- but the names survive in the arrays and the app's existing line-up section
-- keeps rendering.
--
-- That is the whole reason the dual write exists. It is a real cost — two
-- writes for one save — bought deliberately to make this rollback survivable
-- and to keep shipped mobile builds working.
-- ============================================================================

DROP TRIGGER IF EXISTS update_event_lineup_updated_at ON event_lineup;
DROP INDEX IF EXISTS idx_event_lineup_event;
DROP TABLE IF EXISTS event_lineup;