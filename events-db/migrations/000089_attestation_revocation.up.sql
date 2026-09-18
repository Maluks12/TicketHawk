-- ── 000089: Attestation revocation cache ────────────────────────────────────
--
-- Google publishes a list of attestation keys known to be extracted from
-- devices, at https://android.googleapis.com/attestation/status
--
-- A device on that list produces a chain that verifies PERFECTLY to a pinned
-- root, because the key really was issued by Google — it has simply been pulled
-- out of the hardware it was meant to be locked into. So chain verification
-- alone is not enough: without this check, the one class of device most
-- deliberately compromised is the one class that passes.
--
-- Cached in the database rather than held in memory because the list must
-- survive a restart. A deploy during an event would otherwise leave every till
-- unchecked until the first successful fetch, which is precisely when you would
-- not want that.
--
-- FAIL-CLOSED, WITH A WINDOW. If the list cannot be refreshed, enrolment
-- continues against the cached copy until it goes stale (see max_age in the
-- service), then refuses. Failing closed immediately on a network blip would
-- take out enrolment for an outage Google is having; never failing closed means
-- a permanently stale list silently stops being a control.

CREATE TABLE attestation_revocations (
    -- Certificate serial number, lowercase hex, as Google publishes it.
    serial_number VARCHAR(80) PRIMARY KEY,

    -- Google's own vocabulary: REVOKED or SUSPENDED. Both refuse.
    status        VARCHAR(24) NOT NULL,
    reason        VARCHAR(64),
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_attest_revocations_fetched ON attestation_revocations (fetched_at);

-- Single-row bookkeeping for the refresh loop. Kept separate from the entries
-- so "when did we last successfully fetch" survives a list that legitimately
-- comes back empty — which would otherwise be indistinguishable from a failed
-- fetch if we inferred freshness from MAX(fetched_at) over the entries.
CREATE TABLE attestation_revocation_meta (
    id              BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    last_success_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    last_error      TEXT,
    entry_count     INTEGER NOT NULL DEFAULT 0
);

INSERT INTO attestation_revocation_meta (id) VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE attestation_revocations IS
    'Cache of Google''s attestation key revocation list. A device on this list '
    'produces a chain that verifies to a pinned root — the key was genuinely '
    'issued, then extracted from the hardware. Chain verification alone does '
    'not catch it.';