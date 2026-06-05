-- migrations/000036_ad_accounts.down.sql

ALTER TABLE ad_billing_records
    DROP CONSTRAINT IF EXISTS ad_billing_records_account_date_key;

ALTER TABLE ad_billing_records
    ADD CONSTRAINT ad_billing_records_organizer_id_billing_date_key
    UNIQUE (organizer_id, billing_date);

ALTER TABLE ad_billing_records DROP COLUMN IF EXISTS ad_account_id;
ALTER TABLE ad_campaigns       DROP COLUMN IF EXISTS ad_account_id;

DROP TABLE IF EXISTS ad_accounts;
DROP TYPE  IF EXISTS ad_account_status;