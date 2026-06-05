-- ============================================================
-- Migration 000041: Rollback card payment → stitch mandate
-- ============================================================

DROP INDEX IF EXISTS idx_ad_accounts_has_card;

ALTER TABLE ad_accounts
    DROP COLUMN IF EXISTS card_name,
    DROP COLUMN IF EXISTS card_last4,
    DROP COLUMN IF EXISTS card_brand,
    DROP COLUMN IF EXISTS card_expiry;

ALTER TABLE ad_accounts
    ADD COLUMN IF NOT EXISTS stitch_mandate_id     TEXT,
    ADD COLUMN IF NOT EXISTS stitch_mandate_status TEXT NOT NULL DEFAULT 'none'
        CHECK (stitch_mandate_status IN ('none', 'active', 'revoked'));