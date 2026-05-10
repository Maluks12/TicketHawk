-- Make event_id nullable on cart_items so standalone merch (not tied to an event)
-- can be added to the cart. Migration 000028 already made merchandise.event_id optional;
-- this extends the same change to the cart_items table.
ALTER TABLE cart_items ALTER COLUMN event_id DROP NOT NULL;