-- Revert: make event_id NOT NULL again on cart_items.
-- Note: rows with NULL event_id must be deleted first or this will fail.
DELETE FROM cart_items WHERE event_id IS NULL;
ALTER TABLE cart_items ALTER COLUMN event_id SET NOT NULL;