-- migrations/000022_drop_stripe_columns.up.sql
--
-- The project uses Stitch for payments, not Stripe.
-- These columns were carried over from an earlier design and are never
-- populated. Dropping them removes dead schema that would confuse future
-- contributors and code-gen tools.

ALTER TABLE payments
    DROP COLUMN IF EXISTS stripe_customer_id,
    DROP COLUMN IF EXISTS stripe_payment_method_id;