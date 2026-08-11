-- Reverses 000069.
--
-- Any scheduled featuring windows lose their start date and become "featured
-- from now" — the pre-000069 behaviour. is_featured and featured_until are
-- untouched, so nothing stops being featured; scheduled ones simply start
-- immediately again.

DROP INDEX IF EXISTS idx_events_featured_window;

ALTER TABLE events DROP COLUMN IF EXISTS featured_from;