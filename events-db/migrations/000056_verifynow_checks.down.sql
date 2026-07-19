-- migrations/000056_verifynow_checks.down.sql

ALTER TABLE organizers
    DROP COLUMN IF EXISTS bank_account_verified,
    DROP COLUMN IF EXISTS bank_account_verified_at,
    DROP COLUMN IF EXISTS bank_verification_ref,
    DROP COLUMN IF EXISTS dha_id_verified,
    DROP COLUMN IF EXISTS dha_id_verified_at,
    DROP COLUMN IF EXISTS dha_id_check,
    DROP COLUMN IF EXISTS dha_id_checked_at;