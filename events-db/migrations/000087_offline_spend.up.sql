-- ── 000087: Offline spend — policy, attestation, and shortfall handling ─────
--
-- Phase 4. Money moves with no server in the loop, which is the hardest thing
-- this platform does and the reason the architecture doc got a v4.0 revision.
--
-- READ THIS BEFORE ENABLING ANYTHING BELOW.
--
-- Phase 2 established that offline TICKETING needs no keys on the handset: the
-- device checks the synced manifest, and a cloned band that slips through an
-- outage is caught at reconciliation and costs one free entry. That trade was
-- right for admissions and it is WRONG for money.
--
-- Offline spending cannot work that way. To refuse a cloned band with no server
-- to ask, the device must verify the chip's cryptogram itself, which means
-- holding the event's tag key. With symmetric crypto, a verification key is a
-- forgery key — there is no version of this where the device can check a band
-- without also being able to mint one. So enabling offline spend means
-- accepting that a compromised till can forge bands for that event, for that
-- key version.
--
-- Everything in this migration exists to bound that, in three layers:
--
--   1. It is OFF by default, per event. offline_spend_policy defaults to
--      'disabled' and no key ever reaches a device until an organiser turns it
--      on deliberately.
--   2. Keys only reach ATTESTED devices. offline_enabled is gated on
--      hardware-backed attestation, so a rooted handset is restricted to
--      online-only rather than handed the event's key.
--   3. Caps bound the loss when layers 1 and 2 fail. Per-device velocity, a
--      decaying ceiling, a distinct-vendor limit, and a hard zero for
--      cash-funded wallets.
--
-- Layer 3 is the one that matters, because layers 1 and 2 are assurances and
-- layer 3 is arithmetic.

-- ── Per-event offline policy ─────────────────────────────────────────────────

CREATE TYPE offline_spend_policy AS ENUM ('disabled', 'enabled');

CREATE TABLE event_cashless_config (
    event_id UUID PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,

    -- Default 'disabled' is the single most important line in this file. An
    -- organiser who never thinks about offline spend gets an event where a
    -- compromised till cannot forge anything, because no till ever held a key.
    offline_policy offline_spend_policy NOT NULL DEFAULT 'disabled',

    -- §5 velocity cap: max consecutive offline taps and max cumulative offline
    -- spend, per device, before a sync is forced.
    max_offline_taps        INTEGER NOT NULL DEFAULT 3   CHECK (max_offline_taps >= 0),
    max_offline_spend_cents BIGINT  NOT NULL DEFAULT 5000 CHECK (max_offline_spend_cents >= 0),

    -- §5 distinct-vendor cap. The v3.0 design enforced caps per device, which
    -- meant one band tapped at eight stalls during an outage multiplied the
    -- ceiling by eight. This is the per-TAG bound.
    max_distinct_vendors INTEGER NOT NULL DEFAULT 2 CHECK (max_distinct_vendors >= 0),

    -- §5 decaying cap: the longer a device has gone without syncing, the less
    -- it may approve. Stored as JSONB steps rather than columns so the shape can
    -- change without a migration during a build-up:
    --   [{"after_minutes": 10, "max_spend_cents": 2500},
    --    {"after_minutes": 20, "max_spend_cents": 0}]
    -- Degraded connectivity should produce a TIGHTER ceiling, not an equal one.
    decay_steps JSONB NOT NULL DEFAULT
        '[{"after_minutes":10,"max_spend_cents":2500},{"after_minutes":20,"max_spend_cents":0}]',

    -- §5 unrecognised-tag allowance: a band this device has never seen and
    -- cannot look up. Blanket rejection strands legitimate attendees whose band
    -- was issued after the last sync; blind trust is unbounded. A small fixed
    -- allowance is the middle, and zero disables it.
    unknown_tag_allowance_cents BIGINT NOT NULL DEFAULT 500
        CHECK (unknown_tag_allowance_cents >= 0),

    -- How many times reconciliation retries the card before giving up and
    -- routing a negative wallet to a human.
    max_recovery_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_recovery_attempts >= 0),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by UUID REFERENCES users(id)
);

CREATE TRIGGER update_event_cashless_config_updated_at
    BEFORE UPDATE ON event_cashless_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE event_cashless_config IS
    'Per-event offline spend policy. Absent row means offline spend is '
    'disabled — callers must treat a missing row as the safe default, never '
    'as "use the built-in caps".';

-- ── Device attestation ───────────────────────────────────────────────────────
--
-- vendor_pos_devices already carries attestation_status and last_seen_at from
-- 000086. This adds the parts that gate key delivery.

ALTER TABLE vendor_pos_devices
    ADD COLUMN offline_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN attested_at      TIMESTAMPTZ,
    ADD COLUMN attestation_fail_count INTEGER NOT NULL DEFAULT 0,

    -- Public half of the device's hardware-backed signing key (Android
    -- StrongBox where available). Every offline transaction is signed with the
    -- private half, which never leaves the secure element, so a transaction
    -- from a device that has been tampered with is DETECTABLE after the fact
    -- even though it could not be prevented.
    ADD COLUMN signing_pubkey   TEXT;

-- A device may only operate offline while it is attested AND has a key on file.
-- Expressed as a constraint rather than handler logic because it is the gate
-- that decides whether an event's tag key is handed to a physical device
-- somebody else is holding.
ALTER TABLE vendor_pos_devices
    ADD CONSTRAINT chk_pos_offline_requires_attestation
    CHECK (
        offline_enabled = FALSE OR
        (attestation_status = 'passed' AND signing_pubkey IS NOT NULL)
    );

COMMENT ON COLUMN vendor_pos_devices.offline_enabled IS
    'Whether this till may approve spends with no server. Gated on passing '
    'hardware attestation — a device that fails is restricted to online-only '
    'rather than refused outright, so a vendor keeps trading.';

-- ── Offline transaction provenance ───────────────────────────────────────────

ALTER TABLE cashless_transactions
    -- Signature over the transaction by the device's hardware-backed key. NULL
    -- for online transactions, which the server witnessed itself. A missing or
    -- invalid signature on an OFFLINE transaction is a fraud-review trigger,
    -- not a rejection: the attendee already received their drink, and refusing
    -- the record would lose the evidence rather than the loss.
    ADD COLUMN device_signature TEXT,

    -- Per-device monotonic counter, chained by prev_device_hash. This is the
    -- §5 "monotonic sequence counter tied to the device token" and it is what
    -- stops a vendor reordering or backdating their own queue to slip a
    -- transaction under a cap after the fact.
    ADD COLUMN device_seq       BIGINT CHECK (device_seq IS NULL OR device_seq > 0),

    -- On-tag accumulator readings either side of this transaction, for
    -- reconciliation. Lets the server detect a band whose accumulator did not
    -- advance as expected — the signature of a device that skipped the write.
    ADD COLUMN accum_before_cents BIGINT,
    ADD COLUMN accum_after_cents  BIGINT,

    -- When the till approved it, as opposed to created_at (when we received
    -- it). The gap between the two is the outage.
    ADD COLUMN approved_at TIMESTAMPTZ;

-- A device's own queue must be unique and ordered.
CREATE UNIQUE INDEX uq_cashless_device_seq
    ON cashless_transactions (device_id, device_seq)
    WHERE device_id IS NOT NULL AND device_seq IS NOT NULL;

CREATE INDEX idx_cashless_offline_unsigned
    ON cashless_transactions (event_id)
    WHERE mode = 'offline' AND device_signature IS NULL;

-- ── Shortfall state machine ──────────────────────────────────────────────────
--
-- §5 reconciliation. A wallet can only go negative through offline spend, so
-- this table is empty for any event that never enables it.

CREATE TYPE shortfall_state AS ENUM (
    'open',           -- detected, nothing attempted yet
    'recovering',     -- auto-charge in progress / retrying
    'recovered',      -- card charged successfully, wallet square
    'manual_review',  -- retries exhausted, a human owns it
    'investigating',  -- cash-only wallet went negative — see below
    'written_off'     -- grace period elapsed, accepted as a loss
);

CREATE TABLE wallet_shortfalls (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id    UUID NOT NULL REFERENCES cashless_wallets(id) ON DELETE CASCADE,
    event_id     UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,

    -- Positive magnitude of the negative balance when detected.
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    recovered_cents BIGINT NOT NULL DEFAULT 0 CHECK (recovered_cents >= 0),

    state        shortfall_state NOT NULL DEFAULT 'open',
    attempts     INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error   TEXT,

    -- Set when the shortfall is on a wallet that should never have been able to
    -- spend offline at all. A correctly enforced zero cap on a cash-funded
    -- wallet cannot produce a negative balance, so one existing means the cap
    -- was bypassed — a compromised device or a bug, either way not a routine
    -- shortfall to be quietly written off.
    fraud_flag   BOOLEAN NOT NULL DEFAULT FALSE,

    detected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at  TIMESTAMPTZ,

    -- One open shortfall per wallet. A wallet that goes further negative
    -- updates the existing row rather than accumulating a pile of them.
    CONSTRAINT uq_shortfall_open UNIQUE (wallet_id, state) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX idx_shortfalls_event ON wallet_shortfalls (event_id, state);
CREATE INDEX idx_shortfalls_actionable ON wallet_shortfalls (state)
    WHERE state IN ('open', 'recovering');
CREATE INDEX idx_shortfalls_fraud ON wallet_shortfalls (event_id)
    WHERE fraud_flag;

-- ── Offline tap log ──────────────────────────────────────────────────────────
--
-- Refused offline attempts, which never become ledger entries because no money
-- moved. Kept because a band hitting the cap at four stalls in ten minutes is
-- the clearest signal available that something is being worked, and without
-- this table that pattern is invisible — successful spends look identical to
-- ordinary trade.

CREATE TABLE offline_spend_refusals (
    id           UUID PRIMARY KEY,
    event_id     UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    device_id    UUID REFERENCES vendor_pos_devices(id) ON DELETE SET NULL,
    vendor_id    UUID REFERENCES event_vendors(id) ON DELETE SET NULL,
    tag_uid      VARCHAR(20),
    amount_cents BIGINT NOT NULL,
    reason       VARCHAR(64) NOT NULL,
    refused_at   TIMESTAMPTZ NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_offline_refusals_event ON offline_spend_refusals (event_id, refused_at DESC);
CREATE INDEX idx_offline_refusals_tag ON offline_spend_refusals (tag_uid, refused_at DESC)
    WHERE tag_uid IS NOT NULL;