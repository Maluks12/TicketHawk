-- ============================================================================
-- 000092 down.
--
-- Safe. Only notification bookkeeping is lost: what was last announced, the
-- record of notices sent, and any emails still queued. No event, booking or
-- Information data lives in these tables.
--
-- Queued-but-unsent emails ARE lost. Roll back when the queue is empty if that
-- matters:  SELECT count(*) FROM event_update_emails WHERE status IN ('pending','sending');
--
-- Re-applying the up migration re-seeds every live event with its current
-- details as already announced, so a round trip does NOT cause a notification
-- blast. Same property as 000078 and 000082.
-- ============================================================================

DROP TABLE IF EXISTS event_update_emails;
DROP TABLE IF EXISTS event_update_notices;
DROP TABLE IF EXISTS event_detail_watch;