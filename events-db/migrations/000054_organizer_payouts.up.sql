-- migrations/000054_organizer_payouts.up.sql
--
-- Two-tier organizer payout system.
--
--   Tier 'held'    (default): all ticket money lands in the platform Paystack
--                  account. After the event completes, a transfers-based payout
--                  worker settles (subtotal − 6% commission) to the organizer.
--   Tier 'instant': transactions carry a Paystack subaccount split. Paystack
--                  settles the organizer directly on the normal settlement
--                  cycle. The platform's transaction_charge takes its cut PLUS
--                  a 10% rolling reserve of the subtotal, released after the
--                  event completes (Howler-style withholding).
--
-- The organizer_payouts ledger is the single source of truth for what the
-- platform owes and has paid. One row per (event, kind), enforced unique, so
-- the payout worker is idempotent by construction.

-- ── Organizers: payout profile ────────────────────────────────────────────────

ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS payout_tier VARCHAR(10) NOT NULL DEFAULT 'held',
    ADD COLUMN IF NOT EXISTS paystack_subaccount_code VARCHAR(50),
    ADD COLUMN IF NOT EXISTS paystack_recipient_code  VARCHAR(50),
    ADD COLUMN IF NOT EXISTS settlement_bank_code     VARCHAR(20),
    ADD COLUMN IF NOT EXISTS settlement_bank_name     VARCHAR(100),
    ADD COLUMN IF NOT EXISTS bank_account_name        VARCHAR(255),
    ADD COLUMN IF NOT EXISTS subaccount_updated_at    TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS tier_upgraded_at         TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS tier_upgraded_by         UUID REFERENCES users(id);

ALTER TABLE organizers
    ADD CONSTRAINT chk_payout_tier CHECK (payout_tier IN ('held', 'instant'));

-- An organizer on the instant tier must actually have a subaccount to split to.
ALTER TABLE organizers
    ADD CONSTRAINT chk_instant_requires_subaccount
    CHECK (payout_tier <> 'instant' OR paystack_subaccount_code IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_organizers_payout_tier ON organizers(payout_tier);

-- ── Payments: split audit trail ───────────────────────────────────────────────
-- Recorded at initialize time so reconciliation and the reserve-release math
-- never have to re-derive what was sent to Paystack.

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS subaccount_code    VARCHAR(50),
    ADD COLUMN IF NOT EXISTS transaction_charge DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS reserve_amount     DECIMAL(10,2) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_payments_subaccount
    ON payments(subaccount_code) WHERE subaccount_code IS NOT NULL;

-- ── Payout ledger ─────────────────────────────────────────────────────────────

CREATE TABLE organizer_payouts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    organizer_id UUID NOT NULL REFERENCES organizers(id) ON DELETE RESTRICT,
    event_id     UUID NOT NULL REFERENCES events(id)     ON DELETE RESTRICT,

    -- 'event_settlement': tier-1 money the platform collected and owes
    --                     (subtotal − commission over platform-collected bookings).
    -- 'reserve_release' : the 10% reserve withheld from tier-2 split transactions.
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('event_settlement', 'reserve_release')),

    amount   DECIMAL(12,2) NOT NULL CHECK (amount > 0),
    currency VARCHAR(3)    NOT NULL DEFAULT 'ZAR',

    -- pending    → created, waiting for the worker
    -- processing → transfer initiated at Paystack, awaiting confirmation
    -- paid       → transfer succeeded
    -- failed     → transfer failed; surfaced in admin for retry
    status VARCHAR(15) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'paid', 'failed')),

    -- Our idempotency reference sent to Paystack's transfer API. Set once when
    -- the first attempt is made and reused on retries, so a crashed worker can
    -- never double-pay: Paystack rejects a second transfer with the same reference.
    transfer_reference     VARCHAR(64) UNIQUE,
    paystack_transfer_code VARCHAR(50),
    failure_reason         TEXT,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    paid_at    TIMESTAMP WITH TIME ZONE,

    -- Idempotency anchor: at most one settlement and one reserve release per event.
    CONSTRAINT unique_event_payout UNIQUE (event_id, kind)
);

CREATE INDEX idx_organizer_payouts_status    ON organizer_payouts(status);
CREATE INDEX idx_organizer_payouts_organizer ON organizer_payouts(organizer_id);

CREATE TRIGGER update_organizer_payouts_updated_at
    BEFORE UPDATE ON organizer_payouts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();