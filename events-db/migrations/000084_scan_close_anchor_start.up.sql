-- ── Re-anchor scan_close_offset_min from end_time to start_time ──────────────
--
-- scan_close_offset_min used to mean "minutes AFTER end_time that scanning
-- closes". It now means "minutes AFTER start_time" — the same anchor as
-- scan_open_offset_min, so both sides of the admission window are measured from
-- one point and a long event can no longer keep admitting people right through
-- to its end.
--
-- The DEFAULT (NULL) close is no longer a flat offset at all. It lands at
-- whichever comes first:
--
--     start_time + 2h        the ceiling — nobody is admitted more than two
--                            hours into an event, however long it runs
--     end_time   - 30min     for short events, doors shut half an hour before
--                            the event itself is over
--
-- NULL therefore stays NULL here: it means "use the default", and the default
-- is meant to change.
--
-- Rows that already hold a value were written under the OLD meaning, so they
-- must be converted or the window silently moves under live events:
--
--     new_offset = old_offset + event duration in minutes
--
-- which reproduces the exact wall-clock close time the organiser picked. The
-- result is clamped to the existing chk_scan_offsets ceiling of 10080 (7 days);
-- a conversion only reaches that if the event is itself close to a week long,
-- and 7 days after the start is past the end of such an event anyway.
--
-- start_time and end_time are both NOT NULL (000004), so no null guards.

UPDATE events
SET scan_close_offset_min = LEAST(
        scan_close_offset_min
            + GREATEST(CEIL(EXTRACT(EPOCH FROM (end_time - start_time)) / 60)::INTEGER, 0),
        10080
    )
WHERE scan_close_offset_min IS NOT NULL;

COMMENT ON COLUMN events.scan_open_offset_min IS
    'Minutes BEFORE start_time that ticket scanning opens. NULL = default (120).';

COMMENT ON COLUMN events.scan_close_offset_min IS
    'Minutes AFTER start_time that ticket scanning closes. NULL = default rule: '
    '30 minutes before end_time, capped at 2 hours after start_time. '
    'A non-NULL value is taken literally and is NOT capped.';