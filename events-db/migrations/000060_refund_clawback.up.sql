-- migrations/000060_refund_clawback.up.sql
--
-- Event cancellation refunds buyers 100% — subtotal AND service fee — whenever
-- it happens. For a held-tier organiser that costs the platform nothing: the
-- money never left. For an instant-tier organiser it is a real hole, because
-- Paystack settled their share directly at sale time and the withheld reserve
-- was sized for the BUYER refund schedule, which is 0% inside 7 days. A
-- cancellation two days out therefore refunds 100% against a 0% reserve.
--
-- The shortfall is exactly what Paystack paid the organiser:
--     clawback = payments.amount - payments.transaction_charge
--
-- We record that as a debt and net it off whatever the platform owes that
-- organiser next, rather than attempting to reverse a settled Paystack
-- transfer (which is not possible) or chasing them for a payment.

-- ── 1. New ledger kind ────────────────────────────────────────────────────────
-- Kept in organizer_payouts so cancellations appear in the same audit trail as
-- settlements. amount stays POSITIVE and the kind carries the direction: a
-- refund_clawback row means "the organiser owes the platform this much".
ALTER TABLE organizer_payouts
    DROP CONSTRAINT IF EXISTS organizer_payouts_kind_check;

ALTER TABLE organizer_payouts
    ADD CONSTRAINT organizer_payouts_kind_check
    CHECK (kind IN ('event_settlement', 'reserve_release', 'refund_clawback'));

-- Clawback rows are never transferred, so they must never enter the payout
-- worker's queue. They are created already-settled from the worker's point of
-- view; 'paid' would be misleading, so we mark them 'processing' and exclude
-- the kind explicitly in FetchPayableRows.
COMMENT ON COLUMN organizer_payouts.kind IS
    'event_settlement / reserve_release are money OUT to the organiser. '
    'refund_clawback is money the organiser OWES the platform after a '
    'cancellation refund exceeded what was withheld — never transferred, '
    'netted off future payouts via organizers.payout_debt.';

-- ── 2. Running debt balance ───────────────────────────────────────────────────
-- Denormalised deliberately: the payout worker checks this on every row and a
-- SUM over the ledger on each tick would be wasteful. The ledger stays the
-- audit trail; this is the working balance.
ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS payout_debt DECIMAL(12,2) NOT NULL DEFAULT 0;

ALTER TABLE organizers
    DROP CONSTRAINT IF EXISTS chk_payout_debt_non_negative;
ALTER TABLE organizers
    ADD CONSTRAINT chk_payout_debt_non_negative CHECK (payout_debt >= 0);

COMMENT ON COLUMN organizers.payout_debt IS
    'ZAR owed by this organiser to the platform, accrued when a cancellation '
    'refund exceeded the reserve held against it. Deducted from subsequent '
    'payouts before any transfer is initiated. Zero for the vast majority.';

-- ── 3. Subaccount health ──────────────────────────────────────────────────────
-- A subaccount code that no longer resolves at Paystack kills checkout for
-- every event that organiser runs: transaction/initialize returns
-- "404 Invalid Subaccount" and the booking fails outright. Flagging it lets the
-- payment path fall back to a no-split charge and lets admin see who needs
-- re-onboarding, instead of silently losing sales.
ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS subaccount_invalid_at TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN organizers.subaccount_invalid_at IS
    'Set when Paystack rejects the stored subaccount code. Splits are skipped '
    'while non-NULL; cleared when payout details are re-submitted.';

CREATE INDEX IF NOT EXISTS idx_organizers_subaccount_invalid
    ON organizers (subaccount_invalid_at)
    WHERE subaccount_invalid_at IS NOT NULL;

-- ── 4. Dispute log ────────────────────────────────────────────────────────────
-- Chargebacks were previously invisible: the webhook only handled charge.*
-- events, so a dispute never reached the database and the payout ledger would
-- happily settle the organiser their full share on money already reversed.
CREATE TABLE IF NOT EXISTS payment_disputes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id        UUID REFERENCES payments(id) ON DELETE SET NULL,
    booking_id        UUID REFERENCES bookings(id) ON DELETE SET NULL,
    gateway           VARCHAR(20)  NOT NULL DEFAULT 'paystack',
    gateway_reference VARCHAR(255) NOT NULL,
    dispute_reference VARCHAR(255),
    amount            DECIMAL(12,2),
    currency          CHAR(3) NOT NULL DEFAULT 'ZAR',
    status            VARCHAR(40) NOT NULL,
    reason            TEXT,
    raw_payload       JSONB,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_dispute_reference UNIQUE (gateway, gateway_reference)
);

CREATE INDEX IF NOT EXISTS idx_payment_disputes_booking ON payment_disputes (booking_id);
CREATE INDEX IF NOT EXISTS idx_payment_disputes_status  ON payment_disputes (status);