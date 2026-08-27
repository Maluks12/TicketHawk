-- diagnose_reservation_drift.sql
--
-- Read-only. Quantifies how far events.reserved_count and
-- ticket_types.reserved_count have drifted from the live ticket_reservations
-- rows that are supposed to justify them.
--
-- Run BEFORE deploying the repository fix, so you know the size of the
-- backlog, then again AFTER to confirm it stops growing.
--
--   docker run --rm -i --env-file ./.env.production postgres:16-alpine \
--     sh -c 'psql "$DATABASE_URL" -f -' < diagnose_reservation_drift.sql

\echo '=== 1. EVENT-LEVEL DRIFT ==='
-- expected = SUM(quantity) of non-expired reservations for that event.
-- drift > 0 means phantom held capacity: tickets counted as reserved with no
-- reservation row behind them. This is the leak.
SELECT
    e.id,
    e.title,
    e.reserved_count                                AS stored,
    COALESCE(r.expected, 0)                         AS expected,
    e.reserved_count - COALESCE(r.expected, 0)      AS drift,
    e.max_capacity,
    e.tickets_sold
FROM events e
LEFT JOIN (
    SELECT event_id, SUM(quantity)::int AS expected
    FROM ticket_reservations
    WHERE expires_at > NOW()
    GROUP BY event_id
) r ON r.event_id = e.id
WHERE e.reserved_count <> COALESCE(r.expected, 0)
ORDER BY drift DESC
LIMIT 50;

\echo ''
\echo '=== 2. TICKET-TYPE-LEVEL DRIFT ==='
SELECT
    tt.id,
    tt.name,
    e.title                                         AS event_title,
    tt.reserved_count                               AS stored,
    COALESCE(r.expected, 0)                         AS expected,
    tt.reserved_count - COALESCE(r.expected, 0)     AS drift,
    tt.capacity,
    tt.tickets_sold
FROM ticket_types tt
JOIN events e ON e.id = tt.event_id
LEFT JOIN (
    SELECT ticket_type_id, SUM(quantity)::int AS expected
    FROM ticket_reservations
    WHERE expires_at > NOW() AND ticket_type_id IS NOT NULL
    GROUP BY ticket_type_id
) r ON r.ticket_type_id = tt.id
WHERE tt.reserved_count <> COALESCE(r.expected, 0)
ORDER BY drift DESC
LIMIT 50;

\echo ''
\echo '=== 3. FALSELY SOLD OUT (the customer-visible symptom) ==='
-- Rows here are refusing sales purely because of drift.
SELECT
    e.title                                         AS event_title,
    tt.name                                         AS ticket_type,
    tt.capacity,
    tt.tickets_sold,
    tt.reserved_count,
    tt.capacity - tt.tickets_sold - tt.reserved_count AS apparent_available,
    tt.capacity - tt.tickets_sold - COALESCE(r.expected, 0) AS true_available
FROM ticket_types tt
JOIN events e ON e.id = tt.event_id
LEFT JOIN (
    SELECT ticket_type_id, SUM(quantity)::int AS expected
    FROM ticket_reservations
    WHERE expires_at > NOW() AND ticket_type_id IS NOT NULL
    GROUP BY ticket_type_id
) r ON r.ticket_type_id = tt.id
WHERE tt.capacity IS NOT NULL
  AND e.status = 'published'
  AND tt.tickets_sold + tt.reserved_count >= tt.capacity
  AND tt.tickets_sold + COALESCE(r.expected, 0) < tt.capacity
ORDER BY true_available DESC;

\echo ''
\echo '=== 4. CARTS THAT TRIGGER THE MULTI-ROW UPDATE...FROM BUG ==='
-- More than one live reservation row per (cart, event) is the exact shape
-- that UPDATE ... FROM under-releases: Postgres applies one arbitrary match.
SELECT
    cart_id,
    event_id,
    COUNT(*)              AS reservation_rows,
    SUM(quantity)::int    AS total_qty,
    MAX(quantity)::int    AS qty_actually_released_by_old_code,
    SUM(quantity)::int - MAX(quantity)::int AS qty_stranded_on_release
FROM ticket_reservations
GROUP BY cart_id, event_id
HAVING COUNT(*) > 1
ORDER BY qty_stranded_on_release DESC
LIMIT 50;

\echo ''
\echo '=== 5. TOTAL PHANTOM CAPACITY ACROSS THE PLATFORM ==='
SELECT
    SUM(GREATEST(0, e.reserved_count - COALESCE(r.expected, 0))) AS phantom_event_seats
FROM events e
LEFT JOIN (
    SELECT event_id, SUM(quantity)::int AS expected
    FROM ticket_reservations
    WHERE expires_at > NOW()
    GROUP BY event_id
) r ON r.event_id = e.id;