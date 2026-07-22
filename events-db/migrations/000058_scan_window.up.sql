-- ── Custom scan window ────────────────────────────────────────────────────────
-- Organisers can widen (or narrow) the ticket-admission window. Two nullable
-- offset columns, in MINUTES relative to the event's start/end:
--
--   scan_open_offset_min   minutes BEFORE start_time that scanning opens
--   scan_close_offset_min  minutes AFTER  end_time   that scanning closes
--
-- NULL on either column = the historical default (120 min = 2 hours), so
-- existing events and any event that doesn't set a window behave exactly as
-- before. An all-day event just sets a large close offset; a tight door can
-- set 0/0. Values are non-negative; the effective window is always
-- [start - open, end + close].
ALTER TABLE events
    ADD COLUMN IF NOT EXISTS scan_open_offset_min  INTEGER,
    ADD COLUMN IF NOT EXISTS scan_close_offset_min INTEGER;

ALTER TABLE events
    ADD CONSTRAINT chk_scan_offsets
    CHECK (
        (scan_open_offset_min  IS NULL OR scan_open_offset_min  BETWEEN 0 AND 10080) AND
        (scan_close_offset_min IS NULL OR scan_close_offset_min BETWEEN 0 AND 10080)
    );  -- 10080 min = 7 days, a generous ceiling for multi-day festivals