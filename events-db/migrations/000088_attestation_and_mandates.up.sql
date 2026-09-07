-- ── 000088: Device attestation, key delivery, and payment mandates ──────────
--
-- Phase 4b. Two gates, both answering questions Phase 4a deliberately left open:
--
--   1. WHICH DEVICE may hold an event's tag key, and how it gets there without
--      the plaintext ever crossing an untrusted boundary.
--   2. WHETHER a card may be charged to recover a shortfall — which is a
--      consent question, not a technical one.
--
-- Neither gate makes offline spend safe on its own. 4a's caps still do the
-- bounding. These reduce how often the caps are the only thing standing.

-- ── Device attestation ───────────────────────────────────────────────────────
--
-- Every attestation attempt is recorded, passes and failures alike. A till that
-- fails three times in an hour is a device somebody is working on, and that
-- pattern is invisible if only successes are kept.

CREATE TYPE attestation_verdict AS ENUM (
    'passed',
    'failed_integrity',   -- Play Integrity says the device or app is not trustworthy
    'failed_hardware',    -- key is not actually hardware-backed
    'failed_chain',       -- attestation certificate chain did not verify to a pinned root
    'failed_stale',       -- token/nonce too old — replay
    'error'               -- verification could not be completed
);

CREATE TABLE pos_device_attestations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id      UUID NOT NULL REFERENCES vendor_pos_devices(id) ON DELETE CASCADE,
    event_id       UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    verdict        attestation_verdict NOT NULL,

    -- What the verifier concluded, for the audit trail. NOT the raw Play
    -- Integrity token: that is a bearer credential for Google's API and there
    -- is no reason to keep one after it has been evaluated.
    detail         JSONB NOT NULL DEFAULT '{}',

    -- Security level from the Keystore attestation extension: 'strongbox',
    -- 'tee', or 'software'. Recorded even on a pass, because an estate that
    -- silently drifts from strongbox to tee is worth noticing.
    security_level VARCHAR(16),

    attested_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pos_attest_device ON pos_device_attestations (device_id, attested_at DESC);
CREATE INDEX idx_pos_attest_failures ON pos_device_attestations (event_id, attested_at DESC)
    WHERE verdict <> 'passed';

-- ── Attestation nonces ───────────────────────────────────────────────────────
--
-- The server issues a nonce; the device binds it into both the Play Integrity
-- request and the Keystore attestation challenge. Without this, a device could
-- replay a genuine attestation captured from a clean handset — which is exactly
-- what someone rooting a till would do first.
--
-- Single-use and short-lived. consumed_at rather than a delete so a replay
-- attempt is visible rather than merely refused.

CREATE TABLE pos_attestation_nonces (
    nonce       CHAR(64) PRIMARY KEY,          -- hex of 32 random bytes
    device_id   UUID NOT NULL REFERENCES vendor_pos_devices(id) ON DELETE CASCADE,
    issued_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);

CREATE INDEX idx_pos_nonce_device ON pos_attestation_nonces (device_id, issued_at DESC);
CREATE INDEX idx_pos_nonce_expiry ON pos_attestation_nonces (expires_at)
    WHERE consumed_at IS NULL;

-- ── Key delivery ─────────────────────────────────────────────────────────────
--
-- What was released, to which device, under which attestation, and when it
-- stops being valid. The wrapped key itself is NOT stored — it is generated per
-- request, encrypted to that device's attested public key, and handed over
-- once. Keeping a copy would recreate the central store of key material that
-- the whole derive-don't-store design in 000085 exists to avoid.

CREATE TABLE pos_key_grants (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id     UUID NOT NULL REFERENCES vendor_pos_devices(id) ON DELETE CASCADE,
    event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    attestation_id UUID NOT NULL REFERENCES pos_device_attestations(id) ON DELETE RESTRICT,

    key_version   INTEGER NOT NULL,

    -- Expiry is what makes a stolen till a bounded problem. Defaults to the
    -- event's admission window in the issuing code, mirroring how
    -- ScannerCredentialService scopes the gate credential.
    issued_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ,
    revoke_reason TEXT
);

CREATE INDEX idx_pos_key_grants_device ON pos_key_grants (device_id, issued_at DESC);
CREATE INDEX idx_pos_key_grants_live ON pos_key_grants (event_id)
    WHERE revoked_at IS NULL;

COMMENT ON TABLE pos_key_grants IS
    'Audit of event tag keys released to POS devices. The key material itself '
    'is never stored — only the fact of the grant, so a compromise can be '
    'scoped to specific devices and key versions.';

-- ── Payment mandates ─────────────────────────────────────────────────────────
--
-- A recorded, specific permission to charge a card later.
--
-- WHY THIS EXISTS AS A TABLE. cashless_wallets.card_on_file means only "a card
-- top-up succeeded once". That is not permission to charge that card again
-- three weeks later to recover a shortfall, and treating it as such invites
-- exactly the "I didn't authorise this" dispute that 3DS liability shift does
-- NOT cover — that shift applies to fraud, not to disputed authorisation.
--
-- So a mandate stores the consent as evidence: the exact wording shown, when,
-- and the ceiling agreed. If an attendee disputes a recovery charge, this row
-- is the answer.

CREATE TYPE mandate_status AS ENUM ('active', 'revoked', 'expired', 'exhausted');

CREATE TABLE payment_mandates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id   UUID NOT NULL REFERENCES cashless_wallets(id) ON DELETE CASCADE,
    event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Paystack reusable authorization. NOT currently captured anywhere in the
    -- codebase — paystackVerifyData reads only channel/brand/card_type — so the
    -- gateway must be extended to record it. Without this the mandate is a
    -- consent record with nothing to act on.
    authorization_code VARCHAR(120) NOT NULL,
    card_last4         CHAR(4),
    card_brand         VARCHAR(24),

    -- The ceiling the attendee agreed to. A recovery charge above this is
    -- refused regardless of the shortfall, so a runaway offline loss cannot
    -- become an unbounded charge on somebody's card.
    max_charge_cents BIGINT NOT NULL CHECK (max_charge_cents > 0),
    charged_cents    BIGINT NOT NULL DEFAULT 0 CHECK (charged_cents >= 0),

    -- EVIDENCE. The exact text shown at the moment of consent, stored verbatim,
    -- because "we showed them something like this" is not a defence. Versioned
    -- so a wording change does not rewrite what earlier attendees agreed to.
    consent_text     TEXT NOT NULL,
    consent_version  VARCHAR(32) NOT NULL,
    consent_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    consent_ip       INET,

    status      mandate_status NOT NULL DEFAULT 'active',
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_mandate_not_overdrawn CHECK (charged_cents <= max_charge_cents)
);

CREATE INDEX idx_mandates_wallet ON payment_mandates (wallet_id)
    WHERE status = 'active';
CREATE INDEX idx_mandates_expiry ON payment_mandates (expires_at)
    WHERE status = 'active';

COMMENT ON TABLE payment_mandates IS
    'Explicit, recorded permission to charge a stored card later. Required '
    'before any shortfall recovery — cashless_wallets.card_on_file is NOT '
    'consent, it only records that a top-up once succeeded.';

-- ── Shortfall linkage ────────────────────────────────────────────────────────

ALTER TABLE wallet_shortfalls
    ADD COLUMN mandate_id UUID REFERENCES payment_mandates(id) ON DELETE SET NULL,

    -- Set when the attendee was sent a voluntary settle link instead of being
    -- charged. Preferred over a silent charge even where a mandate exists: a
    -- surprise charge on a R30 shortfall costs the disputed amount plus the
    -- chargeback fee plus the relationship.
    ADD COLUMN settle_link_sent_at TIMESTAMPTZ,
    ADD COLUMN settle_link_token   VARCHAR(64);

CREATE UNIQUE INDEX uq_shortfall_settle_token ON wallet_shortfalls (settle_link_token)
    WHERE settle_link_token IS NOT NULL;

-- New state: the attendee has been asked to settle voluntarily and we are
-- waiting. Distinct from manual_review, which means a human on OUR side owns it.
ALTER TYPE shortfall_state ADD VALUE IF NOT EXISTS 'awaiting_settlement';