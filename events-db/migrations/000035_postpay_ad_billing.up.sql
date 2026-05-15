-- ============================================================
-- Migration 000035: Post-pay ad billing + organiser self-registration
--
-- Replaces the pre-paid wallet model with end-of-day automatic
-- billing via a Stitch debit mandate.
--
-- Organiser self-registration:
--   Organisers now register themselves (no admin approval queue).
--   Admin can suspend/ban after the fact if needed.
--
-- Ad billing model change:
--   OLD: organiser pre-loads wallet → campaign draws down balance → pauses at zero
--   NEW: campaign runs all day → nightly worker bills actual spend via Stitch mandate
-- ============================================================

-- 1. Add payment_failed campaign status
ALTER TYPE campaign_status ADD VALUE IF NOT EXISTS 'payment_failed';

-- 2. Store Stitch debit mandate on the organizer row.
--    Set up once by the organiser; used for all future nightly debits.
ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS stitch_mandate_id     TEXT,
    ADD COLUMN IF NOT EXISTS stitch_mandate_status TEXT DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS ad_payment_email      TEXT;

-- 3. Daily billing records — one row per organiser per calendar day billed.
CREATE TABLE IF NOT EXISTS ad_billing_records (
    id                          UUID          PRIMARY KEY DEFAULT uuid_generate_v4(),
    organizer_id                UUID          NOT NULL REFERENCES organizers(id) ON DELETE CASCADE,
    billing_date                DATE          NOT NULL,
    amount                      DECIMAL(10,2) NOT NULL,
    stitch_payment_request_id   TEXT,
    status                      TEXT          NOT NULL DEFAULT 'pending',
    failure_reason              TEXT,
    created_at                  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    completed_at                TIMESTAMPTZ,
    UNIQUE (organizer_id, billing_date),
    CONSTRAINT chk_billing_amount_positive CHECK (amount > 0)
);

CREATE INDEX IF NOT EXISTS idx_billing_records_pending
    ON ad_billing_records(status, billing_date)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_billing_records_organizer
    ON ad_billing_records(organizer_id, billing_date DESC);

-- 4. Add billed flag to ad_daily_spend so the nightly worker knows
--    which rows have already been charged.
ALTER TABLE ad_daily_spend
    ADD COLUMN IF NOT EXISTS billed BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_daily_spend_unbilled
    ON ad_daily_spend(spend_date, billed)
    WHERE billed = FALSE;

-- 5. Raise the minimum daily budget constraint from R50 to R200.
ALTER TABLE ad_campaigns
    DROP CONSTRAINT IF EXISTS chk_campaign_daily_budget_min;
ALTER TABLE ad_campaigns
    ADD CONSTRAINT chk_campaign_daily_budget_min CHECK (daily_budget >= 200);

-- 6. Retire wallet tables (rename, not drop — preserve historical data).
--    Run a dedicated cleanup migration after confirming new system is stable.
ALTER TABLE IF EXISTS ad_wallets       RENAME TO _legacy_ad_wallets;
ALTER TABLE IF EXISTS ad_wallet_topups RENAME TO _legacy_ad_wallet_topups;