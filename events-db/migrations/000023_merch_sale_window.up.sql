-- Adds a time-limited sale window to merchandise items.
-- Both columns are nullable — NULL means "no restriction on that boundary".
-- A NULL sale_start means available immediately.
-- A NULL sale_end   means available indefinitely (existing behaviour).
--
-- The expiry worker calls expireMerchListings() which flips is_active = FALSE
-- when sale_end has passed, keeping query logic simple everywhere else.

ALTER TABLE merchandise
    ADD COLUMN IF NOT EXISTS sale_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS sale_end   TIMESTAMPTZ;

-- Index for the expiry worker: fast lookup of soon-to-expire active items.
CREATE INDEX IF NOT EXISTS idx_merch_sale_end
    ON merchandise(sale_end)
    WHERE is_active = TRUE AND sale_end IS NOT NULL;

-- Partial index: quick lookup of items that are not yet live.
CREATE INDEX IF NOT EXISTS idx_merch_sale_start
    ON merchandise(sale_start)
    WHERE is_active = TRUE AND sale_start IS NOT NULL;