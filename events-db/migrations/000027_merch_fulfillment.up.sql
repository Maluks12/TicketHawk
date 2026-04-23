-- migrations/000027_merch_fulfillment.up.sql
--
-- Adds delivery / pickup fulfillment support to the merch system.
--
-- Three layers of change:
--   1. merchandise — organiser configures what fulfillment options are available
--      per item and optionally charges a delivery fee.
--   2. cart_items  — buyer records their fulfillment choice and delivery address
--      at add-to-cart time.
--   3. merch_orders — fulfillment details are snapshotted at payment confirmation
--      so the organiser always knows how to fulfil each order.
--
-- All columns are nullable / have sensible defaults so existing rows keep
-- working without any data migration.

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. merchandise
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE merchandise
    -- What fulfillment options the organiser makes available for this item.
    -- 'delivery' (default) — buyer must provide a shipping address.
    -- 'pickup'             — buyer collects at a location set by the organiser.
    -- 'both'               — buyer chooses at checkout.
    ADD COLUMN fulfillment_mode VARCHAR(20) NOT NULL DEFAULT 'delivery'
        CHECK (fulfillment_mode IN ('delivery', 'pickup', 'both')),

    -- Human-readable pickup instructions, e.g.
    -- "Main entrance, FNB Stadium — from 17:00 on event day. Bring your ticket."
    -- Required when fulfillment_mode is 'pickup' or 'both'; ignored for delivery-only.
    ADD COLUMN pickup_location TEXT,

    -- Optional flat delivery fee charged once per cart line item (not per unit).
    -- 0.00 = free delivery. Only applied when buyer chooses the delivery method.
    ADD COLUMN delivery_fee NUMERIC(10,2) NOT NULL DEFAULT 0.00
        CHECK (delivery_fee >= 0);

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. cart_items
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE cart_items
    -- Buyer's chosen fulfillment method for this line item.
    -- NULL for ticket items (irrelevant) and legacy merch rows before this migration.
    ADD COLUMN fulfillment_method VARCHAR(20)
        CHECK (fulfillment_method IS NULL OR fulfillment_method IN ('delivery', 'pickup')),

    -- Delivery address as JSONB: {street, suburb, city, province, postal_code}.
    -- Only required when fulfillment_method = 'delivery'.
    ADD COLUMN delivery_address JSONB,

    -- Snapshot of merchandise.delivery_fee at the time the item was added.
    -- Stored separately from unit_price_snapshot so the fee is charged once
    -- per line item, not multiplied by quantity.
    ADD COLUMN delivery_fee_snapshot NUMERIC(10,2) NOT NULL DEFAULT 0.00
        CHECK (delivery_fee_snapshot >= 0);

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. merch_orders
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE merch_orders
    -- Snapshot of the buyer's fulfillment choice at time of payment.
    ADD COLUMN fulfillment_method VARCHAR(20)
        CHECK (fulfillment_method IS NULL OR fulfillment_method IN ('delivery', 'pickup')),

    -- Snapshot of the delivery address for delivery orders.
    ADD COLUMN delivery_address JSONB,

    -- Snapshot of merchandise.pickup_location at time of order for pickup orders.
    -- Preserved even if the organiser later edits the pickup location.
    ADD COLUMN pickup_location TEXT;