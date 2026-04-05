-- migrations/000018_ad_campaigns.up.sql
--
-- Sponsored Events ("Boost") System
--
-- Money flow:
--   Organiser deposits money into ad_wallet (pre-pay via Stitch).
--   They create a campaign on one of their events with a daily_budget.
--   Each feed impression deducts cpm_rate/1000 ZAR from the wallet.
--   At daily_budget → campaign pauses until midnight.
--   At wallet zero  → all organiser campaigns pause until topped up.
--   Higher daily_budget = higher feed rank = more impressions per day.

-- ── Enums ─────────────────────────────────────────────────────────────────────

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'campaign_status') THEN
        CREATE TYPE campaign_status AS ENUM (
            'draft',       -- created but wallet too low to activate
            'active',      -- running, serving impressions
            'paused',      -- manually paused by organiser
            'exhausted',   -- daily budget hit; auto-resumes at midnight
            'ended'        -- end_date passed or manually ended
        );
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'topup_status') THEN
        CREATE TYPE topup_status AS ENUM ('pending', 'completed', 'failed');
    END IF;
END $$;

-- ── Reference number sequence ─────────────────────────────────────────────────
-- Drives TUP-00042 style reference numbers on wallet top-ups.

CREATE SEQUENCE IF NOT EXISTS ad_topup_ref_seq START 1;

-- ── Ad Wallets ────────────────────────────────────────────────────────────────
-- One wallet per organiser. All campaigns draw from the same pool.

CREATE TABLE ad_wallets (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organizer_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    balance      DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    total_topups DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    total_spent  DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    created_at   TIMESTAMPTZ   DEFAULT NOW(),
    updated_at   TIMESTAMPTZ   DEFAULT NOW(),
    CONSTRAINT chk_wallet_balance_non_negative CHECK (balance >= 0)
);

CREATE INDEX idx_ad_wallets_organizer ON ad_wallets(organizer_id);

CREATE TRIGGER update_ad_wallets_updated_at
    BEFORE UPDATE ON ad_wallets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Ad Wallet Top-ups ─────────────────────────────────────────────────────────

CREATE TABLE ad_wallet_topups (
    id                        UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    organizer_id              UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount                    DECIMAL(10,2) NOT NULL,
    stitch_payment_request_id VARCHAR(255),
    reference_number          VARCHAR(20)  UNIQUE NOT NULL,
    status                    topup_status NOT NULL DEFAULT 'pending',
    created_at                TIMESTAMPTZ  DEFAULT NOW(),
    completed_at              TIMESTAMPTZ,
    CONSTRAINT chk_topup_amount_positive CHECK (amount > 0)
);

CREATE INDEX idx_ad_topups_organizer  ON ad_wallet_topups(organizer_id);
CREATE INDEX idx_ad_topups_stitch_req ON ad_wallet_topups(stitch_payment_request_id)
    WHERE stitch_payment_request_id IS NOT NULL;
CREATE INDEX idx_ad_topups_status     ON ad_wallet_topups(status);

-- ── Ad Campaigns ──────────────────────────────────────────────────────────────
-- daily_budget: max ZAR to spend per calendar day (min R50).
-- cpm_rate: ZAR per 1 000 impressions. Platform default R20.
--   R50/day  at R20 CPM = 2 500 impressions/day.
--   R500/day at R20 CPM = 25 000 impressions/day.

CREATE TABLE ad_campaigns (
    id                UUID            PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id          UUID            NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    organizer_id      UUID            NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    daily_budget      DECIMAL(10,2)   NOT NULL,
    cpm_rate          DECIMAL(8,4)    NOT NULL DEFAULT 20.00,
    total_spent       DECIMAL(10,2)   NOT NULL DEFAULT 0.00,
    total_impressions BIGINT          NOT NULL DEFAULT 0,
    total_clicks      BIGINT          NOT NULL DEFAULT 0,
    start_date        DATE            NOT NULL DEFAULT CURRENT_DATE,
    end_date          DATE,
    status            campaign_status NOT NULL DEFAULT 'draft',
    created_at        TIMESTAMPTZ     DEFAULT NOW(),
    updated_at        TIMESTAMPTZ     DEFAULT NOW(),
    CONSTRAINT chk_campaign_daily_budget_min CHECK (daily_budget >= 50),
    CONSTRAINT chk_campaign_end_after_start  CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE INDEX idx_ad_campaigns_organizer ON ad_campaigns(organizer_id);
CREATE INDEX idx_ad_campaigns_event     ON ad_campaigns(event_id);
CREATE INDEX idx_ad_campaigns_status    ON ad_campaigns(status);

-- Hot-path index: active campaigns eligible to serve, ranked by budget
CREATE INDEX idx_ad_campaigns_active_serving
    ON ad_campaigns(daily_budget DESC, event_id)
    WHERE status = 'active';

CREATE TRIGGER update_ad_campaigns_updated_at
    BEFORE UPDATE ON ad_campaigns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Daily Spend ───────────────────────────────────────────────────────────────
-- One row per campaign per day. Upserted on every impression — no cron needed
-- for intra-day tracking. Resets naturally at midnight (new date = new row).

CREATE TABLE ad_daily_spend (
    id           UUID          PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id  UUID          NOT NULL REFERENCES ad_campaigns(id) ON DELETE CASCADE,
    spend_date   DATE          NOT NULL DEFAULT CURRENT_DATE,
    impressions  BIGINT        NOT NULL DEFAULT 0,
    clicks       BIGINT        NOT NULL DEFAULT 0,
    amount_spent DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    UNIQUE (campaign_id, spend_date)
);

CREATE INDEX idx_ad_daily_spend_campaign ON ad_daily_spend(campaign_id);
CREATE INDEX idx_ad_daily_spend_date     ON ad_daily_spend(spend_date);