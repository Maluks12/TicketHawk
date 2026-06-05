-- ============================================================
-- Migration 000041: Replace Stitch mandate with card payment
-- ============================================================

-- Add card payment columns to ad_accounts
ALTER TABLE ad_accounts
    ADD COLUMN IF NOT EXISTS card_name    TEXT,
    ADD COLUMN IF NOT EXISTS card_last4   CHAR(4),
    ADD COLUMN IF NOT EXISTS card_brand   TEXT,
    ADD COLUMN IF NOT EXISTS card_expiry  VARCHAR(5);  -- MM/YY

-- Migrate any accounts that had an active mandate → mark as needing re-registration
-- (stitch mandate IDs are not transferable to card details)
-- No data migration needed — accounts will simply show no card on file.

-- Drop stitch mandate columns (now unused)
ALTER TABLE ad_accounts
    DROP COLUMN IF EXISTS stitch_mandate_id,
    DROP COLUMN IF EXISTS stitch_mandate_status;

-- Update the nightly billing query index
-- has_payment_method is now: card_last4 IS NOT NULL
CREATE INDEX IF NOT EXISTS idx_ad_accounts_has_card
    ON ad_accounts(card_last4)
    WHERE card_last4 IS NOT NULL;