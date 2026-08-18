-- what_is_actually_live.sql
--
-- Grounds the fix priority in real traffic instead of code reading.
-- All 163 bookings bypass reservations; this splits them by the path that
-- actually serves them, and checks whether scanning is in use.
--
--   docker run --rm -i --env-file ./.env.production postgres:16-alpine \
--     sh -c 'psql "$DATABASE_URL" -f -' < what_is_actually_live.sql

\echo '=== A. GUEST vs AUTHENTICATED BOOKINGS ==='
-- is_guest = true  -> served by guest_handler.go (lockout / rdb nil / rate limit)
-- is_guest = false -> authenticated, guest handler irrelevant
SELECT
    is_guest,
    COUNT(*)                                                        AS bookings,
    COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '30 days') AS last_30d,
    MIN(created_at)::date                                           AS first_seen,
    MAX(created_at)::date                                           AS last_seen
FROM bookings
GROUP BY is_guest
ORDER BY bookings DESC;

\echo ''
\echo '=== B. BOOKING STATUS MIX (how many are real completed sales) ==='
SELECT booking_status, payment_status, COUNT(*) AS bookings
FROM bookings
GROUP BY booking_status, payment_status
ORDER BY bookings DESC;

\echo ''
\echo '=== C. IS SCANNING / ADMISSION IN USE? (item 5 relevance) ==='
SELECT 'ticket_assignments total'          AS metric, COUNT(*)::text AS value FROM ticket_assignments
UNION ALL
SELECT 'assignments actually scanned',     COUNT(*)::text FROM ticket_assignments WHERE qr_scanned_at IS NOT NULL
UNION ALL
SELECT 'scanned in last 30 days',          COUNT(*)::text FROM ticket_assignments WHERE qr_scanned_at > NOW() - INTERVAL '30 days'
UNION ALL
SELECT 'scan_idempotency rows',            COUNT(*)::text FROM scan_idempotency;

\echo ''
\echo '=== D. POOL ADMISSIONS (multi-slot bookings = pool scan path) ==='
-- Bookings with >1 assignment are the ones the pool admit path serves.
SELECT
    ticket_quantity > 1 AS is_pool_style,
    COUNT(*)            AS bookings
FROM bookings
WHERE booking_status = 'confirmed'
GROUP BY 1;