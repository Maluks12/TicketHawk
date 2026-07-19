-- migrations/000054_organizer_payouts.down.sql

DROP TABLE IF EXISTS organizer_payouts;

ALTER TABLE payments
    DROP COLUMN IF EXISTS subaccount_code,
    DROP COLUMN IF EXISTS transaction_charge,
    DROP COLUMN IF EXISTS reserve_amount;

ALTER TABLE organizers
    DROP CONSTRAINT IF EXISTS chk_instant_requires_subaccount,
    DROP CONSTRAINT IF EXISTS chk_payout_tier;

DROP INDEX IF EXISTS idx_organizers_payout_tier;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS payout_tier,
    DROP COLUMN IF EXISTS paystack_subaccount_code,
    DROP COLUMN IF EXISTS paystack_recipient_code,
    DROP COLUMN IF EXISTS settlement_bank_code,
    DROP COLUMN IF EXISTS settlement_bank_name,
    DROP COLUMN IF EXISTS bank_account_name,
    DROP COLUMN IF EXISTS subaccount_updated_at,
    DROP COLUMN IF EXISTS tier_upgraded_at,
    DROP COLUMN IF EXISTS tier_upgraded_by;