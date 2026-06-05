-- ============================================================
-- Migration 000039: Ad Accounts (idempotent / safe re-run)
-- ============================================================

-- Safe enum creation
DO $$ BEGIN
  CREATE TYPE ad_account_status AS ENUM ('active', 'disabled');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Ad accounts table
CREATE TABLE IF NOT EXISTS ad_accounts (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organizer_id          UUID        NOT NULL REFERENCES organizers(id) ON DELETE CASCADE,
    name                  TEXT        NOT NULL CHECK (char_length(name) BETWEEN 2 AND 100),
    status                ad_account_status NOT NULL DEFAULT 'active',
    stitch_mandate_id     TEXT,
    stitch_mandate_status TEXT        NOT NULL DEFAULT 'none'
                              CHECK (stitch_mandate_status IN ('none', 'active', 'revoked')),
    ad_payment_email      TEXT,
    currency              CHAR(3)     NOT NULL DEFAULT 'ZAR',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ad_accounts_organizer ON ad_accounts(organizer_id);

-- Link campaigns to an ad account
ALTER TABLE ad_campaigns
    ADD COLUMN IF NOT EXISTS ad_account_id UUID REFERENCES ad_accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_ad_campaigns_account ON ad_campaigns(ad_account_id);

-- Add ad_account_id to billing records
ALTER TABLE ad_billing_records
    ADD COLUMN IF NOT EXISTS ad_account_id UUID REFERENCES ad_accounts(id) ON DELETE SET NULL;

ALTER TABLE ad_billing_records
    DROP CONSTRAINT IF EXISTS ad_billing_records_organizer_id_billing_date_key;

DO $$ BEGIN
  ALTER TABLE ad_billing_records
      ADD CONSTRAINT ad_billing_records_account_date_key
      UNIQUE (ad_account_id, billing_date);
EXCEPTION WHEN duplicate_table THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS idx_ad_billing_account ON ad_billing_records(ad_account_id);

-- Migrate existing organizers that already have a mandate
INSERT INTO ad_accounts (
    organizer_id, name, stitch_mandate_id,
    stitch_mandate_status, ad_payment_email, status
)
SELECT
    id,
    COALESCE(business_name, 'Default Ad Account'),
    stitch_mandate_id,
    COALESCE(stitch_mandate_status, 'none'),
    ad_payment_email,
    'active'
FROM organizers
WHERE stitch_mandate_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Link existing campaigns to the default account
UPDATE ad_campaigns ac
SET    ad_account_id = aa.id
FROM   ad_accounts aa
WHERE  aa.organizer_id = ac.organizer_id
  AND  ac.ad_account_id IS NULL;

-- Link existing billing records to the default account
UPDATE ad_billing_records br
SET    ad_account_id = aa.id
FROM   ad_accounts aa
WHERE  aa.organizer_id = br.organizer_id
  AND  br.ad_account_id IS NULL;