-- ============================================================================
-- 000069: Scheduled featuring window.
--
-- events already has is_featured and featured_until, which express "featured,
-- ending at X" — enough for "feature this now for a week", and not enough for
-- "feature this for the week of the festival".
--
-- The admin console's approval flow now asks for a START date and a duration,
-- so a window needs a start. featured_from supplies it.
--
-- ⚠️  READ THIS BEFORE DEPLOYING — the feed does not honour featured_from yet.
--
-- Whatever selects featured events for the home feed currently tests
-- is_featured (and, where it bothers, featured_until). Adding featured_from
-- without updating that predicate means a scheduled feature goes live the
-- moment it is approved rather than on its start date — which is the exact
-- opposite of what scheduling is for.
--
-- The predicate should become:
--
--     is_featured = TRUE
--     AND (featured_from  IS NULL OR featured_from  <= now())
--     AND (featured_until IS NULL OR featured_until >  now())
--
-- NULL means "no bound" on both sides, so existing rows — which all have
-- featured_from NULL — keep behaving exactly as they do today. That is why the
-- column is nullable with no default rather than defaulting to now().
-- ============================================================================

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS featured_from TIMESTAMPTZ;

COMMENT ON COLUMN events.featured_from IS
    'Start of the featured window. NULL means "featured from now". The feed '
    'must test featured_from <= now() alongside is_featured, or scheduling '
    'has no effect.';

-- Partial index: the feed asks "which events are featured right now", and rows
-- that were never featured are the overwhelming majority.
CREATE INDEX IF NOT EXISTS idx_events_featured_window
    ON events (featured_from, featured_until)
    WHERE is_featured = TRUE;