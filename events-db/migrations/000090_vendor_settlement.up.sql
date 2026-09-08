-- ── 000090: Vendor settlement ───────────────────────────────────────────────
--
-- Vendors take money through tills. Nothing until now worked out what each is
-- owed, so an event could be run and no stall could be paid.
--
-- THE MONEY MODEL, stated because it decides the shape of this table and is
-- easy to get wrong:
--
-- Attendees top up (money in), then spend at vendors (ledger entries moving
-- value from attendee to nowhere in particular). The platform HOLDS the cash
-- the whole time. So a vendor is not receiving money from an attendee — the
-- vendor is claiming, from the platform, the value of sales their till
-- recorded. Settlement is a payout out of float.
--
-- Two consequences that matter:
--
--   1. Unspent balance is a LIABILITY, not revenue. Money topped up and never
--      spent is owed back to the attendee (§6 unspent-balance policy). An
--      organiser who treats the closing float as income is spending other
--      people's money, and this is exactly what the money-transmitter question
--      in §10 turns on.
--
--   2. Settlement is derived, never accumulated. Every figure here is computed
--      from cashless_transactions at settlement time. A running per-vendor
--      total maintained by triggers would drift the first time an offline batch
--      synced late, and drift in a payout figure is money.
--
-- This table is therefore a RECORD OF A CALCULATION, not a balance. Re-running
-- the same period must produce the same numbers; if it does not, the ledger
-- changed and that is worth knowing.

CREATE TYPE settlement_status AS ENUM (
    'draft',      -- calculated, not yet approved by the organiser
    'approved',   -- organiser has signed it off
    'paid',       -- money has left
    'disputed',   -- vendor disagrees; frozen until resolved
    'cancelled'
);

CREATE TABLE vendor_settlements (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    vendor_id  UUID NOT NULL REFERENCES event_vendors(id) ON DELETE RESTRICT,

    -- The window this settlement covers. Half-open [start, end) so consecutive
    -- runs cannot double-count a transaction sitting exactly on a boundary —
    -- which, with offline batches syncing in bursts, is not a rare case.
    period_start TIMESTAMPTZ NOT NULL,
    period_end   TIMESTAMPTZ NOT NULL,

    -- All in cents, all derived. gross is sales minus refunds; commission is
    -- computed on the gross TOTAL rather than per transaction (see
    -- settlement_calc.go for why that matters); net is what the vendor is owed.
    gross_cents      BIGINT NOT NULL,
    refund_cents     BIGINT NOT NULL DEFAULT 0,
    commission_cents BIGINT NOT NULL DEFAULT 0,
    net_cents        BIGINT NOT NULL,

    -- Commission rate as applied, copied from the vendor at calculation time.
    -- Stored rather than joined, because a rate change next month must not
    -- retroactively alter what a settled statement says was charged.
    commission_rate DECIMAL(5,2),

    txn_count        INTEGER NOT NULL DEFAULT 0,
    offline_count    INTEGER NOT NULL DEFAULT 0,

    -- Transactions inside the period that were flagged at ingest — bad
    -- signature, cap violation, sequence gap. Surfaced on the settlement
    -- itself so nobody approves a payout without seeing that some of it is
    -- disputed. Not deducted automatically: whether a flagged sale is real is a
    -- judgement, and quietly withholding a vendor's money on a heuristic is the
    -- wrong default.
    flagged_count    INTEGER NOT NULL DEFAULT 0,
    flagged_cents    BIGINT NOT NULL DEFAULT 0,

    status     settlement_status NOT NULL DEFAULT 'draft',
    reference  VARCHAR(64),          -- bank/EFT reference once paid
    note       TEXT,

    calculated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at   TIMESTAMPTZ,
    paid_at       TIMESTAMPTZ,
    created_by    UUID REFERENCES users(id),

    CONSTRAINT chk_settlement_period CHECK (period_end > period_start),
    CONSTRAINT chk_settlement_amounts CHECK (
        gross_cents >= 0 AND commission_cents >= 0 AND
        -- The invariant the whole calculation rests on: not one cent created or
        -- lost between gross and net.
        net_cents = gross_cents - commission_cents
    ),

    -- One settlement per vendor per period. Re-running a period must update the
    -- draft rather than create a second one somebody might also pay.
    CONSTRAINT uq_settlement_vendor_period UNIQUE (vendor_id, period_start, period_end)
);

CREATE INDEX idx_settlements_event ON vendor_settlements (event_id, status);
CREATE INDEX idx_settlements_vendor ON vendor_settlements (vendor_id, period_end DESC);
CREATE INDEX idx_settlements_unpaid ON vendor_settlements (event_id)
    WHERE status IN ('draft', 'approved');

COMMENT ON TABLE vendor_settlements IS
    'A record of a calculation, not a balance. Every figure is derived from '
    'cashless_transactions at settlement time — re-running a period must '
    'reproduce the same numbers, and if it does not, the ledger changed.';

-- ── Float reconciliation ─────────────────────────────────────────────────────
--
-- A point-in-time snapshot of where an event's cashless money actually sits.
-- Written when an organiser runs a reconciliation, so the figures they acted on
-- are recoverable later even as the ledger keeps moving.

CREATE TABLE event_float_reconciliations (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,

    topups_cents     BIGINT NOT NULL,  -- money in, card + cash
    purchases_cents  BIGINT NOT NULL,  -- money spent at vendors
    refunds_cents    BIGINT NOT NULL,
    payouts_cents    BIGINT NOT NULL,  -- unspent balance already returned

    -- What attendees are still owed. THE number that matters: it is a
    -- liability, not revenue, and §6 commits to returning or rolling it over.
    outstanding_liability_cents BIGINT NOT NULL,

    -- Wallets currently negative — only possible through offline spend.
    shortfall_cents  BIGINT NOT NULL DEFAULT 0,
    shortfall_count  INTEGER NOT NULL DEFAULT 0,

    settled_cents    BIGINT NOT NULL DEFAULT 0,  -- already paid to vendors
    unsettled_cents  BIGINT NOT NULL DEFAULT 0,  -- owed to vendors

    reconciled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reconciled_by UUID REFERENCES users(id)
);

CREATE INDEX idx_float_recon_event ON event_float_reconciliations (event_id, reconciled_at DESC);

COMMENT ON COLUMN event_float_reconciliations.outstanding_liability_cents IS
    'Money attendees topped up and have not spent. A LIABILITY owed back under '
    'the §6 unspent-balance policy — never revenue, and the figure the '
    'money-transmitter analysis in §10 turns on.';