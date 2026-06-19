DROP INDEX IF EXISTS idx_payments_gateway_reference;

ALTER TABLE payments
    DROP COLUMN IF EXISTS gateway_reference,
    DROP COLUMN IF EXISTS gateway_redirect_url;