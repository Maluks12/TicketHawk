-- scan_path_check.sql
--
-- scan_idempotency is empty despite 17 scanned assignments. This distinguishes
-- two very different explanations:
--
--   (1) the pool admit path was never used  -> item 5 is latent
--   (2) pool admits ran and SaveScanIdempotency failed silently (the discarded
--       `_ =` error) -> item 5 is LIVE and retries have been double-admitting
--
--   docker run --rm -i --env-file ./.env.production postgres:16-alpine \
--     sh -c 'psql "$DATABASE_URL" -f -' < scan_path_check.sql

\echo '=== A. SCANNED ASSIGNMENTS BY STATUS ==='
-- The pool admit path updates rows with status = 'unassigned'.
-- Any scanned row still sitting at 'unassigned' was admitted via the POOL path,
-- and therefore SHOULD have produced a scan_idempotency row.
SELECT
    ta.status,
    COUNT(*)                          AS scanned_rows,
    MIN(ta.qr_scanned_at)::date       AS first_scan,
    MAX(ta.qr_scanned_at)::date       AS last_scan
FROM ticket_assignments ta
WHERE ta.qr_scanned_at IS NOT NULL
GROUP BY ta.status
ORDER BY scanned_rows DESC;

\echo ''
\echo '=== B. SCANNED ROWS ON POOL-STYLE BOOKINGS (the smoking gun) ==='
-- Rows here with status = 'unassigned' mean the pool path definitely ran.
-- If this count is > 0 while scan_idempotency is empty, the idempotency save
-- has been failing silently on every admit.
SELECT
    b.booking_number,
    b.ticket_quantity,
    COUNT(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL)                             AS scanned,
    COUNT(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL AND ta.status = 'unassigned') AS scanned_via_pool,
    MAX(ta.qr_scanned_at)                                                            AS last_scan
FROM bookings b
JOIN ticket_assignments ta ON ta.booking_id = b.id
WHERE b.ticket_quantity > 1
GROUP BY b.id, b.booking_number, b.ticket_quantity
HAVING COUNT(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL) > 0
ORDER BY last_scan DESC;

\echo ''
\echo '=== C. DOES THE TABLE EVEN EXIST / IS IT WRITABLE? ==='
-- Rules out "migration never applied" as the reason it is empty.
SELECT
    to_regclass('public.scan_idempotency') IS NOT NULL AS table_exists,
    (SELECT COUNT(*) FROM information_schema.columns
      WHERE table_name = 'scan_idempotency')           AS column_count;

SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name = 'scan_idempotency'
ORDER BY ordinal_position;

\echo ''
\echo '=== D. OVER-ADMISSION CHECK (did any booking admit more than it bought?) ==='
-- The consequence you would actually care about if (2) is true.
SELECT
    b.booking_number,
    b.ticket_quantity                                              AS purchased,
    COUNT(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL)           AS admitted,
    COUNT(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL) - b.ticket_quantity AS over_admitted
FROM bookings b
JOIN ticket_assignments ta ON ta.booking_id = b.id
GROUP BY b.id, b.booking_number, b.ticket_quantity
HAVING COUNT(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL) > b.ticket_quantity
ORDER BY over_admitted DESC;