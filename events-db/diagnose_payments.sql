-- diagnose_payments.sql
-- Inspect where recent bookings are stuck. Run read-only; changes nothing.
--
-- PowerShell:
--   Get-Content scripts/diagnose_payments.sql | docker compose exec -T db psql -U devuser -d events_dev
--
-- What to look for:
--   * payment_gateway = 'paypal' AND p.status = 'pending' AND gateway_reference IS NOT NULL
--     => the PayPal order was created but never captured (the bug we just fixed
--        on /payments/return). These will NOT appear in the tickets tab because
--        isRealTicket hides non-confirmed/non-completed bookings.
--   * booking_status = 'pending' while p.status = 'completed'
--     => payment captured but the booking never got confirmed (worker/sync issue).

-- 1) The 25 most recent bookings with their payment row, side by side.
SELECT
    b.created_at::timestamptz(0)        AS created,
    left(b.id::text, 8)                 AS booking,
    b.booking_number                    AS bk_no,
    b.booking_status                    AS bk_status,
    b.payment_status                    AS bk_pay_status,
    p.payment_gateway                   AS gateway,
    p.payment_method                    AS method,
    p.status                            AS pay_status,
    (p.gateway_reference IS NOT NULL)   AS has_ref,
    b.total_amount                      AS amount,
    b.currency                          AS ccy
FROM bookings b
LEFT JOIN payments p ON p.booking_id = b.id
ORDER BY b.created_at DESC
LIMIT 25;

-- 2) Count of likely-stuck PayPal orders: created (have a reference) but pending.
SELECT
    count(*) AS stuck_paypal_pending
FROM bookings b
JOIN payments p ON p.booking_id = b.id
WHERE p.payment_gateway = 'paypal'
  AND p.status IN ('pending', 'processing')
  AND p.gateway_reference IS NOT NULL;

-- 3) The actual stuck PayPal rows, with their order reference, so you can decide
--    whether to re-pay or purge them.
SELECT
    b.created_at::timestamptz(0)  AS created,
    b.booking_number              AS bk_no,
    left(b.id::text, 8)           AS booking,
    p.status                      AS pay_status,
    b.booking_status              AS bk_status,
    p.gateway_reference           AS paypal_order_id,
    b.total_amount                AS amount
FROM bookings b
JOIN payments p ON p.booking_id = b.id
WHERE p.payment_gateway = 'paypal'
  AND p.status IN ('pending', 'processing')
ORDER BY b.created_at DESC;