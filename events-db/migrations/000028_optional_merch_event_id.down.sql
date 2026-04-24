-- Revert: event_id becomes required again.
-- WARNING: This will fail if any standalone merch (NULL event_id) exists.
-- Delete or reassign standalone merch before running this rollback.
ALTER TABLE merchandise ALTER COLUMN event_id SET NOT NULL;