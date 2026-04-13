DROP INDEX IF EXISTS idx_merch_sale_end;
DROP INDEX IF EXISTS idx_merch_sale_start;

ALTER TABLE merchandise
    DROP COLUMN IF EXISTS sale_start,
    DROP COLUMN IF EXISTS sale_end;