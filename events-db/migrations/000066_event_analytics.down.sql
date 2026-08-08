-- Reverses 000066.
--
-- Dropping the table loses collected reach/view/dwell history permanently —
-- there is no other copy, and Redis only ever holds the last ~2 minutes. Take a
-- dump first if the data matters.
--
-- Nothing else depends on this table: events.views, scan_events and
-- ticket_assignments are untouched by 000066, so the dashboard falls back to
-- exactly the metrics it had before.

DROP INDEX IF EXISTS idx_emb_time;
DROP INDEX IF EXISTS idx_emb_event_time;
DROP TABLE IF EXISTS event_metric_buckets;