-- ============================================================
-- Migration 000045: Revert ad billing to a pre-paid wallet
--
-- Replaces the post-pay end-of-day debit model with a per-organiser
-- pre-paid wallet:
--   * One wallet per organiser (keyed by users.id, matching
--     ad_campaigns.organizer_id).
--   * Organisers top up the wallet by card via Paystack.
--   * Each impression draws the wallet down; at zero balance all of
--     that organiser's campaigns pause until topped up.
--   * Minimum daily budget stays R200 (unchanged from migration 000035).
--
-- Post-pay artifacts are retired rename-don't-drop where they hold real
-- history (ad_billing_records), and dropped where they are display-only
-- metadata (ad_account card columns).
-- ============================================================

-- ── Top-up status enum (originally created in 000018) ─────────────────────────
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'topup_status') THEN
        CREATE TYPE topup_status AS ENUM ('pending', 'completed', 'failed');
    END IF;
END $$;

-- Human-friendly top-up reference numbers (TUP-00001).
CREATE SEQUENCE IF NOT EXISTS ad_topup_ref_seq START 1;

-- ── Ad wallets (one per organiser) ────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ad_wallets (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organizer_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    balance      DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    total_topups DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    total_spent  DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_wallet_balance_non_negative CHECK (balance >= 0)
);

CREATE INDEX IF NOT EXISTS idx_ad_wallets_organizer ON ad_wallets(organizer_id);

DROP TRIGGER IF EXISTS update_ad_wallets_updated_at ON ad_wallets;
CREATE TRIGGER update_ad_wallets_updated_at
    BEFORE UPDATE ON ad_wallets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Wallet top-ups (card only, paid via Paystack) ─────────────────────────────
CREATE TABLE IF NOT EXISTS ad_wallet_topups (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organizer_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount             DECIMAL(10,2) NOT NULL,
    paystack_reference VARCHAR(255) UNIQUE NOT NULL,
    reference_number   VARCHAR(20) UNIQUE,
    status             topup_status NOT NULL DEFAULT 'pending',
    created_at         TIMESTAMPTZ DEFAULT NOW(),
    completed_at       TIMESTAMPTZ,
    CONSTRAINT chk_topup_amount_positive CHECK (amount > 0)
);

CREATE INDEX IF NOT EXISTS idx_ad_topups_organizer ON ad_wallet_topups(organizer_id);
CREATE INDEX IF NOT EXISTS idx_ad_topups_reference ON ad_wallet_topups(paystack_reference);
CREATE INDEX IF NOT EXISTS idx_ad_topups_status    ON ad_wallet_topups(status);

-- ── Restore balances from the legacy wallet (if 000035 renamed it) ────────────
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables WHERE table_name = '_legacy_ad_wallets'
    ) THEN
        INSERT INTO ad_wallets (organizer_id, balance, total_topups, total_spent)
        SELECT organizer_id, balance, total_topups, total_spent
        FROM   _legacy_ad_wallets
        ON CONFLICT (organizer_id) DO NOTHING;
    END IF;
END $$;

-- ── Retire post-pay billing records (preserve history) ────────────────────────
ALTER TABLE IF EXISTS ad_billing_records RENAME TO _legacy_ad_billing_records;

-- ── Drop per-account card-on-file columns (display-only metadata) ─────────────
-- Campaigns now draw from the per-organiser wallet, not a stored card.
ALTER TABLE ad_accounts
    DROP COLUMN IF EXISTS card_name,
    DROP COLUMN IF EXISTS card_last4,
    DROP COLUMN IF EXISTS card_brand,
    DROP COLUMN IF EXISTS card_expiry,
    DROP COLUMN IF EXISTS ad_payment_email;