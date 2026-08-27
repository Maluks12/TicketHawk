-- Reverses 000075.
--
-- Postgres cannot DROP a value from an enum. Removing 'event_edit' would mean
-- recreating staff_request_kind and rewriting every dependent column, which
-- risks far more than the value costs to leave in place.
--
-- The value is inert once no code creates requests of that kind, so the down
-- migration deliberately does nothing but record why.
--
-- If a rollback must also clear the rows:
--
--   DELETE FROM staff_requests
--    WHERE kind = 'event_edit' AND status = 'pending_approval';

SELECT 1;