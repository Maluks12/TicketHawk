-- migrations/000019_booking_email_retries.up.sql
--
-- Durable retry queue for booking confirmation emails.
--
-- When the booking worker fails to send a confirmation email it previously
-- launched a fire-and-forget goroutine that would be silently lost on any
-- process restart before the 3-second timer fired.
--
-- This table makes retries survive crashes:
--   1. On email failure the worker INSERTs a row here instead of spawning a
--      goroutine.
--   2. On startup the worker drains all pending rows before accepting new jobs.
--   3. A periodic background drain runs every 5 minutes to catch rows added
--      while the server is running.
--   4. After max_attempts the row is marked 'failed' and left for ops review.

CREATE TABLE IF NOT EXISTS booking_email_retries (
    id          UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    booking_id  UUID         NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,

    -- How many send attempts have been made so far (0 = never attempted after
    -- the initial failure that created this row).
    attempts    SMALLINT     NOT NULL DEFAULT 0,

    -- Hard cap — after this many attempts the row is marked failed and skipped.
    max_attempts SMALLINT    NOT NULL DEFAULT 5,

    -- Exponential back-off: the worker will not retry until this time passes.
    retry_after  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Terminal states: 'pending' → being retried, 'sent' → succeeded,
    -- 'failed' → exhausted max_attempts.
    status       VARCHAR(10)  NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'sent', 'failed')),

    -- Capture the last error message for ops visibility.
    last_error   TEXT,

    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_booking_email_retries_booking ON booking_email_retries(booking_id);

-- The drain query only touches pending rows whose back-off has elapsed.
CREATE INDEX idx_booking_email_retries_drain
    ON booking_email_retries(retry_after)
    WHERE status = 'pending';

CREATE TRIGGER update_booking_email_retries_updated_at
    BEFORE UPDATE ON booking_email_retries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();