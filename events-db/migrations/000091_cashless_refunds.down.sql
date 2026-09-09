-- Reverse 000091.
--
-- Dropping refunds_txn_id severs the link between every refund and the sale it
-- reversed. The refund entries themselves survive — they are ordinary ledger
-- rows and balances stay correct — but nothing afterwards can tell which sale
-- each one reversed, or stop a vendor refunding the same purchase twice.
--
-- Dropping `reason` discards why every manual adjustment was made, which is the
-- only answer available when somebody asks later.
--
-- Safe on a scratch database. After any real refund, take a dump first.

DROP INDEX IF EXISTS idx_cashless_refunds_original;

ALTER TABLE cashless_transactions
    DROP CONSTRAINT IF EXISTS chk_adjustment_has_reason,
    DROP CONSTRAINT IF EXISTS chk_refund_has_original;

ALTER TABLE cashless_transactions
    DROP COLUMN IF EXISTS reason,
    DROP COLUMN IF EXISTS refunds_txn_id;