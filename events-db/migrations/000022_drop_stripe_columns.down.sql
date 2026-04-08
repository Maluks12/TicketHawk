-- migrations/000022_drop_stripe_columns.down.sql

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS stripe_customer_id       VARCHAR(255),
    ADD COLUMN IF NOT EXISTS stripe_payment_method_id VARCHAR(255);