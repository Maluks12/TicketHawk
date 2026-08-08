-- ============================================================================
-- 000066: Event analytics — reach, views and dwell.
--
-- WHAT WAS MISSING
--
-- The organiser dashboard could answer "how much did I earn" but not "how many
-- people did this reach, and how many of them cared". events.views existed as a
-- single lifetime counter with no time dimension, so there was no way to ask
-- "what happened in the hour after I posted the link" — the question every
-- organiser actually asks.
--
-- WHY MINUTE BUCKETS, NOT RAW ROWS
--
-- One row per impression would be the flexible choice and the wrong one. A
-- single event on the homepage for a week generates millions of rows to answer
-- questions that are always aggregate. Instead we pre-aggregate to the minute —
-- the finest grain the dashboard exposes — and roll up to hour/day on read with
-- date_trunc. A busy event costs at most 1 440 rows/day, and hour/day queries
-- stay fast without a second rollup table to keep in sync.
--
-- Nothing here is destructive: events.views keeps being written exactly as
-- before, so every existing surface (trending score, event page counter) is
-- untouched. This table is additive and starts empty — historical reach is
-- genuinely unknowable, and the dashboard says so rather than back-filling a
-- guess.
--
-- WHAT IS *NOT* HERE
--
-- Scanned tickets and attendance need no new storage: scan_events (000051) and
-- ticket_assignments.qr_scanned_at (000042) already carry timestamps, so both
-- are derived at read time and can never drift from the source of truth.
-- Likewise PSR is subtotal - organizer_fee on bookings, already stored.
-- ============================================================================

CREATE TABLE IF NOT EXISTS event_metric_buckets (
    event_id      UUID        NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    -- Truncated to the minute, always UTC. Display-zone bucketing (SAST days)
    -- happens on read via AT TIME ZONE so one stored row serves every zone.
    bucket_at     TIMESTAMPTZ NOT NULL,

    -- Reach: the event's card was returned in a feed/search/category response.
    impressions   BIGINT      NOT NULL DEFAULT 0,
    -- Interest: someone opened the event detail page.
    views         BIGINT      NOT NULL DEFAULT 0,

    -- Dwell, reported by clients when the detail screen is closed.
    -- Retention = dwell_ms / views. dwell_samples is kept alongside so the
    -- dashboard can show HOW MUCH of the view population actually reported a
    -- dwell — without it, a 5% sample and a 95% sample look identical, and the
    -- organiser has no way to know whether to trust the number.
    dwell_ms      BIGINT      NOT NULL DEFAULT 0,
    dwell_samples BIGINT      NOT NULL DEFAULT 0,

    CONSTRAINT event_metric_buckets_pk PRIMARY KEY (event_id, bucket_at),
    CONSTRAINT emb_non_negative CHECK (
        impressions >= 0 AND views >= 0 AND dwell_ms >= 0 AND dwell_samples >= 0
    )
);

-- Per-event time-range scans: every dashboard query is
-- "WHERE event_id = $1 AND bucket_at BETWEEN $2 AND $3". The PK already leads
-- with event_id so it serves this directly; declared explicitly for clarity of
-- intent and so a future PK change can't silently drop the access path.
CREATE INDEX IF NOT EXISTS idx_emb_event_time
    ON event_metric_buckets (event_id, bucket_at DESC);

-- Cross-event sweeps: the organiser-wide overview aggregates every event they
-- own over a window, and retention housekeeping deletes by age.
CREATE INDEX IF NOT EXISTS idx_emb_time
    ON event_metric_buckets (bucket_at DESC);

COMMENT ON TABLE event_metric_buckets IS
    'Minute-grain reach/view/dwell counters per event. Written by the Redis '
    'flush worker (60s) with a direct-upsert fallback when Redis is down. '
    'Rolled up to hour/day at read time.';