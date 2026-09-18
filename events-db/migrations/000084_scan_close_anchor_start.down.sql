-- Re-anchor scan_close_offset_min back to end_time.
--
-- Lossy in one direction only: any row the up-migration clamped at the 10080
-- ceiling cannot be recovered exactly. Floored at 0 because the old meaning
-- ("minutes after end_time") could not express a close before the event ended.

UPDATE events
SET scan_close_offset_min = GREATEST(
        scan_close_offset_min
            - GREATEST(CEIL(EXTRACT(EPOCH FROM (end_time - start_time)) / 60)::INTEGER, 0),
        0
    )
WHERE scan_close_offset_min IS NOT NULL;

COMMENT ON COLUMN events.scan_open_offset_min IS NULL;
COMMENT ON COLUMN events.scan_close_offset_min IS NULL;