-- Reverse migration 000035
ALTER TABLE ad_daily_spend DROP COLUMN IF EXISTS billed;

DROP TABLE IF EXISTS ad_billing_records;

ALTER TABLE IF EXISTS _legacy_ad_wallet_topups RENAME TO ad_wallet_topups;
ALTER TABLE IF EXISTS _legacy_ad_wallets       RENAME TO ad_wallets;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS stitch_mandate_id,
    DROP COLUMN IF EXISTS stitch_mandate_status,
    DROP COLUMN IF EXISTS ad_payment_email;

-- Restore old minimum budget constraint
ALTER TABLE ad_campaigns
    DROP CONSTRAINT IF EXISTS chk_campaign_daily_budget_min;
ALTER TABLE ad_campaigns
    ADD CONSTRAINT chk_campaign_daily_budget_min CHECK (daily_budget >= 50);

-- campaign_status enum values ('payment_failed') cannot be removed in Postgres.