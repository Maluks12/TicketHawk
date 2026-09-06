-- ── 000085: RFID/NFC tag registry ───────────────────────────────────────────
--
-- Phase 1 of the cashless programme. This migration adds tag IDENTITY only:
-- which physical chip belongs to which event, what it is bound to, and whether
-- it may be trusted. No money moves through anything created here — the wallet
-- ledger lands in Phase 3 and deliberately depends on the trust classification
-- established below.
--
-- THREE THINGS THAT DIFFER FROM THE v4.0 ARCHITECTURE DOC, on purpose:
--
--  1. There is no per-tag `crypto_key` column. Section 4 of the doc lists one,
--     but storing a key per tag turns the tags table into the single highest-
--     value target on the platform: one dump and every wristband at every event
--     is forgeable, and there is no recovery short of re-encoding the estate.
--     Keys are DERIVED instead (NXP AN10922-style AES-CMAC diversification)
--     from a platform master held only in the environment, so the database
--     holds no key material at all. `key_version` is what is stored — enough to
--     re-derive, useless on its own. Rotating a compromised master is then a
--     version bump rather than a data migration.
--
--  2. Chips are classified by HOW THEY CAN BE VERIFIED, not by price tier. The
--     doc splits "premium" (DESFire) from "budget" (NTAG 424) and treats the
--     premium tier as strictly stronger. For SERVER-side verification the
--     opposite is true, and the distinction matters more than the price:
--
--       server_verified  — NTAG 424 DNA. Every tap emits a fresh SDM/SUN
--                          message the BACKEND can verify by itself. The
--                          reader is not trusted; it only relays.
--       reader_verified  — DESFire EV2/EV3. Mutual authentication happens
--                          between chip and reader. Cryptographically stronger
--                          at the tap, but the server sees only the reader's
--                          CLAIM that it succeeded. Trust therefore rests on
--                          device attestation, which is Phase 4 work.
--       unverified       — UID-only chips (MIFARE Classic, NTAG21x). Trivially
--                          cloneable. Permitted for low-value access-control
--                          pilots and NOTHING else.
--
--     This is why the classification is a stored column with a CHECK rather
--     than a comment: Phase 3 gates wallet binding on it, so the constraint
--     has to exist before any money does.
--
--  3. scan_events gets a `medium` column rather than a new `scan_mode` value.
--     scan_mode is ('online','offline') — that is TRANSPORT, and a tap can
--     arrive either way just as a QR scan can. Adding 'nfc' to that enum would
--     have quietly changed the meaning of every existing query that filters on
--     it, including the attendance sum in analytics_repository and the
--     "delivered paid event" check in payout_repository.CheckInstantEligibility
--     that gates instant-tier promotion. Medium and transport are orthogonal
--     and are stored that way.

-- ── Enums ────────────────────────────────────────────────────────────────────

CREATE TYPE tag_chip_type AS ENUM (
    'ntag424',      -- NTAG 424 DNA — SDM/SUN, server-verifiable
    'desfire_ev2',  -- MIFARE DESFire EV2 — diversified keys, reader-verified
    'desfire_ev3',  -- MIFARE DESFire EV3 — as EV2, plus Transaction MAC
    'uid_only'      -- MIFARE Classic / NTAG21x — access-control pilots ONLY
);

CREATE TYPE tag_verification_class AS ENUM (
    'server_verified',
    'reader_verified',
    'unverified'
);

CREATE TYPE tag_status AS ENUM (
    'provisioned',  -- known to the platform, not yet encoded with keys
    'encoded',      -- keys written to the chip, not yet handed to an attendee
    'active',       -- bound and in the field
    'deactivated',  -- retired normally (event over, tag reclaimed)
    'lost',         -- reported lost/stolen — must never be admitted again
    'destroyed'     -- physically damaged, written off
);

-- ── Batches ──────────────────────────────────────────────────────────────────
--
-- Wristbands arrive and are encoded in runs, not one at a time. The batch is
-- the unit an operator actually handles, and it is also the unit the per-tag
-- software fee in §6 of the doc is invoiced against — which is why the billing
-- columns sit here rather than being recomputed from a COUNT(*) at invoice
-- time. `activated_count` is maintained by trigger below so the billable figure
-- can never drift from the tags themselves.

CREATE TABLE rfid_tag_batches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    label           VARCHAR(120) NOT NULL,
    chip_type       tag_chip_type NOT NULL,
    key_version     INTEGER NOT NULL DEFAULT 1 CHECK (key_version >= 1),
    expected_count  INTEGER NOT NULL DEFAULT 0 CHECK (expected_count >= 0),
    activated_count INTEGER NOT NULL DEFAULT 0 CHECK (activated_count >= 0),
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_rfid_batches_event ON rfid_tag_batches (event_id);

CREATE TRIGGER update_rfid_tag_batches_updated_at
    BEFORE UPDATE ON rfid_tag_batches
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Tags ─────────────────────────────────────────────────────────────────────

CREATE TABLE rfid_tags (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id           UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    batch_id           UUID REFERENCES rfid_tag_batches(id) ON DELETE SET NULL,

    -- Chip UID as uppercase hex, no separators. 7 bytes on every chip we
    -- support, but 4-byte and 10-byte UIDs exist in the wild, so the column is
    -- sized for 10 and the length is constrained rather than fixed.
    tag_uid            VARCHAR(20) NOT NULL,

    chip_type          tag_chip_type NOT NULL,
    verification_class tag_verification_class NOT NULL,
    key_version        INTEGER NOT NULL DEFAULT 1 CHECK (key_version >= 1),
    status             tag_status NOT NULL DEFAULT 'provisioned',

    -- What the tag admits. Mirrors the two token families the QR path already
    -- has: an individual assignment, or a booking pool. Exactly one, or
    -- neither while the tag is unbound — enforced below.
    assignment_id      UUID REFERENCES ticket_assignments(id) ON DELETE SET NULL,
    booking_id         UUID REFERENCES bookings(id) ON DELETE SET NULL,

    -- SDM read counter high-water mark (NTAG 424 only; NULL elsewhere).
    -- The chip increments this on every SDM read and it never resets, so
    -- "strictly greater than what we last saw" is the anti-replay control: a
    -- captured tap payload replayed later carries a counter we have already
    -- seen and is refused. This single column is what makes a server_verified
    -- tap safe to accept over an untrusted reader.
    last_read_ctr      BIGINT CHECK (last_read_ctr >= 0),

    bound_at           TIMESTAMPTZ,
    activated_at       TIMESTAMPTZ,
    deactivated_at     TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A physical chip belongs to one event at a time. Reuse across events is a
    -- Phase 5 concern (re-encode + re-register); until then this is the
    -- guardrail that stops last weekend's wristband opening this weekend's gate.
    CONSTRAINT uq_rfid_tag_event_uid UNIQUE (event_id, tag_uid),

    CONSTRAINT chk_rfid_uid_hex
        CHECK (tag_uid ~ '^[0-9A-F]{8,20}$' AND length(tag_uid) % 2 = 0),

    -- Bound to an assignment or a pool, never both.
    CONSTRAINT chk_rfid_single_binding
        CHECK (assignment_id IS NULL OR booking_id IS NULL),

    -- An active tag must be bound to something. Without this a tag could sit in
    -- 'active' with no binding and the tap path would have to invent a verdict.
    CONSTRAINT chk_rfid_active_is_bound
        CHECK (status <> 'active' OR assignment_id IS NOT NULL OR booking_id IS NOT NULL),

    -- The classification must match the chip. Stored rather than derived so the
    -- Phase 3 wallet check is a column read on the hot path, but constrained so
    -- the two can never disagree.
    CONSTRAINT chk_rfid_class_matches_chip CHECK (
        (chip_type = 'ntag424'     AND verification_class = 'server_verified') OR
        (chip_type IN ('desfire_ev2','desfire_ev3') AND verification_class = 'reader_verified') OR
        (chip_type = 'uid_only'    AND verification_class = 'unverified')
    ),

    -- A read counter only means anything on a chip that emits one.
    CONSTRAINT chk_rfid_ctr_only_sdm
        CHECK (last_read_ctr IS NULL OR chip_type = 'ntag424')
);

-- Hot path: every tap is a lookup by (event, uid).
CREATE INDEX idx_rfid_tags_event_uid ON rfid_tags (event_id, tag_uid);

-- "Which tag is this ticket on?" — the reverse lookup the organiser console
-- and the re-issue flow both need.
CREATE INDEX idx_rfid_tags_assignment ON rfid_tags (assignment_id)
    WHERE assignment_id IS NOT NULL;
CREATE INDEX idx_rfid_tags_booking ON rfid_tags (booking_id)
    WHERE booking_id IS NOT NULL;

-- Manifest delta sync pulls tags changed since a cursor (Phase 2).
CREATE INDEX idx_rfid_tags_event_updated ON rfid_tags (event_id, updated_at);

-- One live tag per assignment. Re-issuing a lost wristband means deactivating
-- the old row first, which is exactly the sequence we want to force: the old
-- chip stops working before the new one starts.
CREATE UNIQUE INDEX uq_rfid_active_assignment
    ON rfid_tags (assignment_id)
    WHERE assignment_id IS NOT NULL AND status IN ('encoded', 'active');

CREATE TRIGGER update_rfid_tags_updated_at
    BEFORE UPDATE ON rfid_tags
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Lifecycle audit ──────────────────────────────────────────────────────────
--
-- Append-only, same philosophy as scan_events: a tag's history is a sequence of
-- facts, not a mutable status column. When an attendee disputes an admission or
-- (from Phase 3) a charge, this is the record that answers "what was this chip
-- allowed to do, and since when".

CREATE TYPE tag_event_kind AS ENUM (
    'provisioned', 'encoded', 'bound', 'unbound',
    'activated', 'deactivated', 'reported_lost', 'key_rotated', 'tap_refused'
);

CREATE TABLE rfid_tag_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tag_id     UUID NOT NULL REFERENCES rfid_tags(id) ON DELETE CASCADE,
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    kind       tag_event_kind NOT NULL,
    actor_id   UUID REFERENCES users(id),   -- NULL when the system acted
    device_id  VARCHAR(64),
    detail     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_rfid_tag_events_tag ON rfid_tag_events (tag_id, created_at DESC);
CREATE INDEX idx_rfid_tag_events_event ON rfid_tag_events (event_id, created_at DESC);

-- ── Billable-count trigger ───────────────────────────────────────────────────
--
-- §6 charges per wristband "successfully transitioned to active status", so the
-- counter moves on the transition INTO active and never on the way out: a tag
-- that was live and has since been deactivated was still billable. Written as a
-- trigger rather than application code because the fee is the revenue line, and
-- a count maintained in one of four handlers is a count that eventually drifts.

CREATE OR REPLACE FUNCTION rfid_bump_batch_activated() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.batch_id IS NOT NULL
       AND NEW.status = 'active'
       AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM 'active')
    THEN
        UPDATE rfid_tag_batches
        SET activated_count = activated_count + 1
        WHERE id = NEW.batch_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rfid_bump_batch_activated
    AFTER INSERT OR UPDATE OF status ON rfid_tags
    FOR EACH ROW EXECUTE FUNCTION rfid_bump_batch_activated();

-- ── scan_events: medium + tag ────────────────────────────────────────────────

CREATE TYPE scan_medium AS ENUM ('qr', 'nfc');

ALTER TABLE scan_events
    ADD COLUMN medium  scan_medium NOT NULL DEFAULT 'qr',
    ADD COLUMN tag_uid VARCHAR(20);

COMMENT ON COLUMN scan_events.medium IS
    'How the credential was presented (qr | nfc). Orthogonal to mode, which is '
    'transport (online | offline). Existing rows default to qr, which is what '
    'they were.';

COMMENT ON COLUMN scan_events.tag_uid IS
    'Chip UID for nfc scans; NULL for qr. Denormalised from rfid_tags on '
    'purpose — a refused tap may reference a UID with no tag row at all, and '
    'that is precisely the row an investigation needs to keep.';

CREATE INDEX idx_scan_events_tag ON scan_events (tag_uid)
    WHERE tag_uid IS NOT NULL;