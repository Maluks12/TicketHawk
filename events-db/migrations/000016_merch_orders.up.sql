-- Migration: 000016_merch_orders.up.sql
--
-- Creates the merch_orders table to record every merchandise sale at the point
-- of payment confirmation. Previously, DecrementVariantStock was defined but
-- never called, so no sale record existed after the cart was cleaned up.
--
-- Design decisions:
--   - Orders are only inserted when a payment webhook confirms success, never
--     speculatively at checkout. Stock is decremented atomically with insertion.
--   - fulfillment_status distinguishes 'fulfilled' (normal) from 'oversold'
--     (payment captured but stock ran out — needs manual resolution). This can
--     happen because merch stock is not pre-reserved at checkout.
--   - Unique constraint on (stitch_payment_req_id, variant_id) provides
--     idempotency: duplicate webhook deliveries are silently ignored via
--     ON CONFLICT DO NOTHING.
--   - cart_id and user_id are SET NULL on delete so order history survives
--     cart cleanup and user deletion.

CREATE TABLE merch_orders (
    id                     UUID         PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Cart that originated this purchase. SET NULL when cart is cleaned up
    -- by the expiry worker — the order record survives.
    cart_id                UUID         REFERENCES carts(id) ON DELETE SET NULL,

    -- Product references
    merchandise_id         UUID         NOT NULL REFERENCES merchandise(id),
    variant_id             UUID         NOT NULL REFERENCES merchandise_variants(id),
    event_id               UUID         NOT NULL REFERENCES events(id),

    -- Buyer — mutually exclusive: registered users have user_id, guests have session_id
    user_id                UUID         REFERENCES users(id) ON DELETE SET NULL,
    session_id             UUID,        -- guest buyer identifier

    -- Purchase details (snapshot at time of sale)
    quantity               INTEGER      NOT NULL CHECK (quantity > 0),
    unit_price             NUMERIC(10,2) NOT NULL CHECK (unit_price >= 0),
    total_amount           NUMERIC(10,2) NOT NULL CHECK (total_amount >= 0),

    -- Payment linkage
    stitch_payment_req_id  TEXT         NOT NULL,

    -- 'fulfilled'  — stock decremented successfully
    -- 'oversold'   — payment captured but stock was 0 at fulfillment time;
    --                admin must resolve manually (refund or source stock)
    fulfillment_status     VARCHAR(20)  NOT NULL DEFAULT 'fulfilled'
                           CHECK (fulfillment_status IN ('fulfilled', 'oversold')),

    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Idempotency: one row per variant per payment request.
    -- Duplicate webhook deliveries hit ON CONFLICT DO NOTHING.
    CONSTRAINT uq_merch_order_payment_variant
        UNIQUE (stitch_payment_req_id, variant_id)
);

-- Stats queries filter by event_id most often
CREATE INDEX idx_merch_orders_event    ON merch_orders(event_id);
-- Dashboard user order history
CREATE INDEX idx_merch_orders_user     ON merch_orders(user_id) WHERE user_id IS NOT NULL;
-- Webhook reconciliation via cart
CREATE INDEX idx_merch_orders_cart     ON merch_orders(cart_id) WHERE cart_id IS NOT NULL;
-- Oversold alert queries
CREATE INDEX idx_merch_orders_oversold ON merch_orders(fulfillment_status)
    WHERE fulfillment_status = 'oversold';
-- Revenue reporting by date
CREATE INDEX idx_merch_orders_created  ON merch_orders(created_at DESC);