-- migrations/000056_verifynow_checks.up.sql
--
-- VerifyNow verification results (South Africa DHA + bank-account ownership).
--
-- Two paid checks, run at two different moments and stored as RESULTS only —
-- never the raw inputs:
--
--   Bank-account ownership — runs at bank-detail entry (SetPayoutDetails),
--     the one moment the full account number is in hand before it goes to
--     Paystack. We store a boolean + timestamp and discard the number. This
--     is why the check lives at entry, not at promotion: by promotion time
--     the number is already gone (we only keep last 4).
--
--   DHA ID (said_verification) — runs at Tier-2 promotion. Confirms the SA ID
--     resolves at Home Affairs and the returned name matches the organizer's
--     legal name. A full match auto-approves the instant tier.
--
-- Grandfathering: existing organizers are NULL/false on all columns. The bank
-- check populates next time they save payout details; the ID check populates
-- at their next promotion attempt. Nothing is back-filled.

ALTER TABLE organizers
    -- Bank-account ownership (verified at detail entry)
    ADD COLUMN IF NOT EXISTS bank_account_verified      BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS bank_account_verified_at   TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS bank_verification_ref       VARCHAR(80),  -- provider transaction id, audit trail

    -- DHA ID check (verified at promotion)
    ADD COLUMN IF NOT EXISTS dha_id_verified            BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS dha_id_verified_at         TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS dha_id_check               JSONB,        -- raw said_verification response, audit trail
    ADD COLUMN IF NOT EXISTS dha_id_checked_at          TIMESTAMP WITH TIME ZONE;