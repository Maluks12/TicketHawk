-- ============================================================================
-- 000051: Offline scanning — the scan-event ledger.
--
-- Scans stop being "an UPDATE that either happened or didn't" and become
-- append-only EVENTS flowing in from devices (online scans included), which a
-- server-side reconciler applies to ticket_assignments. Client-generated ids
-- give exact-once ingestion across flaky reconnects (ON CONFLICT DO NOTHING),
-- and first-received-at-server wins on conflicts: later duplicates are kept as
-- 'collision' rows pointing at the winner — the multi-gate exceptions report.
--
-- updated_at triggers on ticket_assignments/bookings already exist (000005 /
-- 000042 era), so manifest delta sync needs no further schema support.
-- ============================================================================

CREATE TYPE scan_mode    AS ENUM ('online', 'offline');
CREATE TYPE scan_verdict AS ENUM ('admitted', 'duplicate', 'collision', 'rejected', 'provisional');

CREATE TABLE scan_events (
    id             UUID PRIMARY KEY,                 -- CLIENT-generated = idempotency key
    event_id       UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    assignment_id  UUID REFERENCES ticket_assignments(id) ON DELETE SET NULL, -- NULL for pool admits / unknown refs
    booking_id     UUID REFERENCES bookings(id) ON DELETE SET NULL,
    raw_ref        VARCHAR(64),                      -- assignment/booking id AS CLAIMED by the device,
                                                     -- kept even when it doesn't resolve (provisionals)
    admit_count    INT NOT NULL DEFAULT 1 CHECK (admit_count > 0),  -- >1 only for pool admits
    scanned_by     UUID NOT NULL REFERENCES users(id),
    device_id      VARCHAR(64) NOT NULL,             -- app-install UUID
    mode           scan_mode    NOT NULL,
    verdict        scan_verdict NOT NULL,            -- SERVER-decided, not the device's claim
    scanned_at     TIMESTAMPTZ NOT NULL,             -- device clock: display/reporting only
    received_at    TIMESTAMPTZ NOT NULL DEFAULT now(), -- server clock: the authority ordering
    conflict_with  UUID REFERENCES scan_events(id)   -- on collision: the scan that won
);

CREATE INDEX idx_scan_events_event  ON scan_events (event_id, received_at DESC);
CREATE INDEX idx_scan_events_assign ON scan_events (assignment_id) WHERE assignment_id IS NOT NULL;
CREATE INDEX idx_scan_events_flags  ON scan_events (event_id)
    WHERE verdict IN ('collision', 'provisional');