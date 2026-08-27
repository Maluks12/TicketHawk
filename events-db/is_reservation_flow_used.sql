-- is_reservation_flow_used.sql
--
-- The drift diagnostic only reports MISMATCHES, so a code path that has never
-- run reads as 0 = 0 and looks healthy. This establishes whether the
-- reservation/checkout flow is exercised in production at all.
--
--   docker run --rm -i --env-file ./.env.production postgres:16-alpine \
--     sh -c 'psql "$DATABASE_URL" -f -' < is_reservation_flow_used.sql

\echo '=== A. IS THERE ANY RESERVATION STATE AT ALL? ==='
SELECT 'ticket_reservations rows (all)'        AS metric, COUNT(*)::text AS value FROM ticket_reservations
UNION ALL
SELECT 'ticket_reservations rows (live)',      COUNT(*)::text FROM ticket_reservations WHERE expires_at > NOW()
UNION ALL
SELECT 'ticket_reservations WITH ticket_type', COUNT(*)::text FROM ticket_reservations WHERE ticket_type_id IS NOT NULL
UNION ALL
SELECT 'events  with reserved_count > 0',      COUNT(*)::text FROM events        WHERE reserved_count > 0
UNION ALL
SELECT 'ticket_types with reserved_count > 0', COUNT(*)::text FROM ticket_types  WHERE reserved_count > 0
UNION ALL
SELECT 'ticket_types total',                   COUNT(*)::text FROM ticket_types;

\echo ''
\echo '=== B. WHICH BOOKING PATH IS ACTUALLY USED? ==='
-- cart_id NOT NULL  => came through cart/checkout (uses reservations)
-- cart_id NULL      => direct or guest booking (bypasses reservations entirely)
SELECT
    CASE WHEN cart_id IS NULL THEN 'direct / guest (no reservations)'
         ELSE 'cart checkout (uses reservations)' END AS booking_path,
    COUNT(*)                          AS bookings,
    MIN(created_at)::date             AS first_seen,
    MAX(created_at)::date             AS last_seen
FROM bookings
GROUP BY 1
ORDER BY bookings DESC;

\echo ''
\echo '=== C. CART-CHECKOUT BOOKINGS INVOLVING A TICKET TYPE ==='
-- These are the only bookings that could ever have leaked
-- ticket_types.reserved_count. If this is 0, the leak has had no opportunity
-- to fire yet -- which is a statement about traffic, not about correctness.
SELECT
    COUNT(*)                                          AS cart_bookings_with_ticket_type,
    COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '30 days') AS in_last_30_days
FROM bookings
WHERE cart_id IS NOT NULL AND ticket_type_id IS NOT NULL;

\echo ''
\echo '=== D. MULTI-TICKET-TYPE CARTS (triggers the UPDATE...FROM bug) ==='
-- Live cart_items, independent of whether reservations currently exist.
SELECT
    cart_id,
    event_id,
    COUNT(DISTINCT ticket_type_id) AS distinct_ticket_types,
    SUM(quantity)::int             AS total_qty
FROM cart_items
WHERE item_type = 'ticket'
GROUP BY cart_id, event_id
HAVING COUNT(DISTINCT ticket_type_id) > 1
ORDER BY distinct_ticket_types DESC
LIMIT 20;