-- Make event_id optional on merchandise so merch can exist without being
-- linked to a specific event (standalone / general store merch).
ALTER TABLE merchandise ALTER COLUMN event_id DROP NOT NULL;