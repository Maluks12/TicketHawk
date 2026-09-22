ALTER TABLE organizers DROP CONSTRAINT IF EXISTS chk_custom_service_fee;
ALTER TABLE organizers DROP CONSTRAINT IF EXISTS chk_custom_commission_pct;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS custom_fees_updated_by,
    DROP COLUMN IF EXISTS custom_fees_updated_at,
    DROP COLUMN IF EXISTS custom_fees_note,
    DROP COLUMN IF EXISTS custom_service_fee,
    DROP COLUMN IF EXISTS custom_commission_pct;

COMMENT ON COLUMN organizers.commission_rate IS NULL;