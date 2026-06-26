-- 000047_pool_scan_idempotency.up.sql
--
-- Group-pool admissions admit N heads per scan (a counter-like action), so unlike
-- the boolean single-ticket scan they are NOT naturally idempotent: a retried
-- "admit 3" would admit 3 more. This table records the result of each admit
-- against a client-supplied idempotency key so a network retry returns the same
-- result instead of admitting again.
--
-- Rows are disposable (safe to prune by created_at); they only need to outlive a
-- request retry window.

CREATE TABLE IF NOT EXISTS scan_idempotency (
    idempotency_key UUID PRIMARY KEY,
    booking_id      UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    admitted_count  INTEGER NOT NULL DEFAULT 0,
    scanned_by      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_scan_idempotency_created
    ON scan_idempotency (created_at);