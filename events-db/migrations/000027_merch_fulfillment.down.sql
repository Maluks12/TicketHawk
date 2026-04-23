-- migrations/000027_merch_fulfillment.down.sql
ALTER TABLE merch_orders  DROP COLUMN IF EXISTS pickup_location;
ALTER TABLE merch_orders  DROP COLUMN IF EXISTS delivery_address;
ALTER TABLE merch_orders  DROP COLUMN IF EXISTS fulfillment_method;

ALTER TABLE cart_items    DROP COLUMN IF EXISTS delivery_fee_snapshot;
ALTER TABLE cart_items    DROP COLUMN IF EXISTS delivery_address;
ALTER TABLE cart_items    DROP COLUMN IF EXISTS fulfillment_method;

ALTER TABLE merchandise   DROP COLUMN IF EXISTS delivery_fee;
ALTER TABLE merchandise   DROP COLUMN IF EXISTS pickup_location;
ALTER TABLE merchandise   DROP COLUMN IF EXISTS fulfillment_mode;