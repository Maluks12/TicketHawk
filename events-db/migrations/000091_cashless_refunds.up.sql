-- ── 000091: Refund linkage and adjustments ──────────────────────────────────
--
-- cashless_transactions has had a 'refund' kind since 000086 and nothing could
-- create one. A bar that cannot reverse a mistyped R500 charge finds that out
-- on its first night, with a queue.
--
-- WHY A COLUMN RATHER THAN detail JSONB. A refund must be tied to the sale it
-- reverses, and that link is load-bearing: it is what stops a vendor refunding
-- R500 against a R50 sale, and what lets AuthorizeRefund see how much of an
-- original has already been given back. A foreign key gets that checked by the
-- database and indexed for the lookup; a JSONB field gets it checked by
-- whichever handler remembers to.

ALTER TABLE cashless_transactions
    -- The sale this reverses. NULL for everything that is not a refund.
    ADD COLUMN refunds_txn_id UUID REFERENCES cashless_transactions(id) ON DELETE RESTRICT,

    -- Mandatory free text for refunds and adjustments. Not decoration: an
    -- attendee disputing a charge weeks later, or an auditor asking why a
    -- manager moved R200, needs the answer attached to the entry rather than
    -- reconstructed from who was on shift.
    ADD COLUMN reason TEXT;

-- ON DELETE RESTRICT above, not CASCADE, is deliberate. Deleting a purchase
-- that has been refunded would orphan the refund and silently leave the
-- attendee credited with money against nothing. The ledger is append-only, so
-- this should never fire — but if something ever tries, failing loudly beats
-- quietly breaking the arithmetic.

-- Refunds must reference an original; nothing else may.
ALTER TABLE cashless_transactions
    ADD CONSTRAINT chk_refund_has_original CHECK (
        (kind = 'refund' AND refunds_txn_id IS NOT NULL) OR
        (kind <> 'refund' AND refunds_txn_id IS NULL)
    );

-- A manual adjustment without a stated reason is an unexplained movement of
-- somebody else's money.
ALTER TABLE cashless_transactions
    ADD CONSTRAINT chk_adjustment_has_reason CHECK (
        kind <> 'adjustment' OR (reason IS NOT NULL AND length(trim(reason)) > 0)
    );

-- The lookup AuthorizeRefund depends on: how much of this sale has already been
-- given back. Partial refunds mean several rows may point at one original, so
-- this is not unique.
CREATE INDEX idx_cashless_refunds_original ON cashless_transactions (refunds_txn_id)
    WHERE refunds_txn_id IS NOT NULL;

COMMENT ON COLUMN cashless_transactions.refunds_txn_id IS
    'The purchase this refund reverses. Several partial refunds may reference '
    'one sale; their total may never exceed it (enforced in AuthorizeRefund).';