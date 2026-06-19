-- ============================================================
-- Migration 000044: Gateway-neutral payment columns
--
-- Stitch is being removed in favour of Paystack (cards + Apple Pay)
-- and PayPal. The payments table previously stored Stitch-specific
-- identifiers; this migration adds provider-neutral columns that both
-- new gateways write to, and backfills them from the legacy columns so
-- historical rows stay reconcilable. The old stitch_* columns are left
-- in place and dropped in a later migration once the new system is stable.
-- ============================================================

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS gateway_reference    VARCHAR(255),
    ADD COLUMN IF NOT EXISTS gateway_redirect_url TEXT;

-- Backfill from the legacy Stitch columns.
UPDATE payments
SET    gateway_reference    = COALESCE(gateway_reference, stitch_payment_request_id),
       gateway_redirect_url = COALESCE(gateway_redirect_url, stitch_payment_url)
WHERE  stitch_payment_request_id IS NOT NULL;

-- Hot-path lookup used by the webhook handlers to resolve a payment by its
-- provider reference (Paystack reference / PayPal order id).
CREATE INDEX IF NOT EXISTS idx_payments_gateway_reference
    ON payments(gateway_reference)
    WHERE gateway_reference IS NOT NULL;