-- purge_unpaid_bookings.sql
--
-- Removes bookings that never became real tickets: failed, declined, abandoned
-- and still-pending payments. Mirrors exactly what the Tickets page now hides
-- (isRealTicket). Paid tickets — confirmed, refunded, or paid-then-cancelled —
-- are KEPT.
--
-- FK notes: ticket_assignments + email_retries are ON DELETE CASCADE, and
-- payments.booking_id is ON DELETE SET NULL, so nothing blocks the delete. We
-- remove the matching payment rows first so the payments table stays tidy.
--
-- Run:
--   docker compose exec db psql -U devuser -d events_dev -f - < scripts/purge_unpaid_bookings.sql
-- or paste into:  docker compose exec db psql -U devuser -d events_dev

-- 1) DRY RUN — see what WOULD be deleted before committing to anything.
SELECT booking_status, payment_status, COUNT(*) AS n
FROM bookings
WHERE booking_status NOT IN ('confirmed', 'refunded')
  AND payment_status  NOT IN ('completed', 'refunded')
GROUP BY booking_status, payment_status
ORDER BY n DESC;

-- 2) PURGE — wrapped in a transaction so it's all-or-nothing.
BEGIN;

DELETE FROM payments p
USING bookings b
WHERE p.booking_id = b.id
  AND b.booking_status NOT IN ('confirmed', 'refunded')
  AND b.payment_status  NOT IN ('completed', 'refunded');

DELETE FROM bookings
WHERE booking_status NOT IN ('confirmed', 'refunded')
  AND payment_status  NOT IN ('completed', 'refunded');

COMMIT;