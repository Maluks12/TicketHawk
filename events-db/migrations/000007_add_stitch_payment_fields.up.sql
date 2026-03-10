-- migrations/000007_add_stitch_payment_fields.up.sql
-- Add Stitch-specific columns to support Apple Pay, Google Pay, Samsung Pay, and PayPal
-- via the Stitch payment gateway.

-- Add Stitch columns to payments table
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS stitch_payment_request_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS stitch_payment_url         TEXT,
    ADD COLUMN IF NOT EXISTS stitch_linked_account_id   VARCHAR(255);

-- Add unique constraint so each booking has at most one active payment record
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'uq_payments_booking_id'
    ) THEN
        ALTER TABLE payments
        ADD CONSTRAINT uq_payments_booking_id UNIQUE (booking_id);
    END IF;
END $$;

-- Index for Stitch payment request lookups (used by the webhook handler)
CREATE INDEX IF NOT EXISTS idx_payments_stitch_request_id
    ON payments(stitch_payment_request_id)
    WHERE stitch_payment_request_id IS NOT NULL;

-- Extend payment_method_type enum with instant_eft if it doesn't exist yet
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumtypid = 'payment_method_type'::regtype
          AND enumlabel = 'instant_eft'
    ) THEN
        ALTER TYPE payment_method_type ADD VALUE 'instant_eft';
    END IF;
END $$;
