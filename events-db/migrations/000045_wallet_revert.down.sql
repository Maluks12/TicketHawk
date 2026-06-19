-- Best-effort reversal. Card columns are restored empty (display metadata was
-- not preserved); historical billing rows are restored from the legacy table.

ALTER TABLE ad_accounts
    ADD COLUMN IF NOT EXISTS card_name        TEXT,
    ADD COLUMN IF NOT EXISTS card_last4       CHAR(4),
    ADD COLUMN IF NOT EXISTS card_brand       TEXT,
    ADD COLUMN IF NOT EXISTS card_expiry      VARCHAR(5),
    ADD COLUMN IF NOT EXISTS ad_payment_email TEXT;

ALTER TABLE IF EXISTS _legacy_ad_billing_records RENAME TO ad_billing_records;

DROP TABLE IF EXISTS ad_wallet_topups;
DROP TABLE IF EXISTS ad_wallets;

DROP SEQUENCE IF EXISTS ad_topup_ref_seq;
-- topup_status enum is intentionally left in place (may predate this migration).