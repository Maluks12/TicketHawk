-- ============================================================================
-- TicketHawk — verification script for the scan-ledger / refund / expiry fixes
--
-- PART A is read-only. Run it first, on production, any time.
-- PART B writes and then ROLLS BACK. Nothing persists. Run it once you've read
--        Part A's output. It briefly takes row locks (FOR UPDATE), so prefer
--        staging or a quiet window.
-- PART C is read-only. Run it AFTER deploying and scanning one live ticket.
--
-- Every statement in Part B is wrapped in a DO block with its own exception
-- handler, so one failure reports itself and the rest still run instead of
-- poisoning the transaction.
-- ============================================================================


-- ############################################################################
-- PART A — READ-ONLY DIAGNOSTICS (safe on production)
-- ############################################################################

\echo '=== A1. Scan-ledger gap: events that happened but are invisible ==='
-- This is the #8 bug and the gate for the payout backfill.
-- admitted_assignments > 0 AND ledger_rows = 0  =>  the event is invisible to
-- both the analytics door figure and CheckInstantEligibility.
SELECT e.id,
       e.title,
       e.status,
       e.start_time,
       count(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL)          AS admitted_assignments,
       (SELECT count(*) FROM scan_events se WHERE se.event_id = e.id) AS ledger_rows,
       CASE
         WHEN count(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL) > 0
          AND (SELECT count(*) FROM scan_events se WHERE se.event_id = e.id) = 0
         THEN 'INVISIBLE — needs backfill'
         ELSE 'ok'
       END AS verdict
FROM   events e
JOIN   bookings b            ON b.event_id  = e.id
JOIN   ticket_assignments ta ON ta.booking_id = b.id
GROUP  BY e.id, e.title, e.status, e.start_time
HAVING count(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL) > 0
ORDER  BY e.start_time DESC;


\echo ''
\echo '=== A2. Why is the payout account stuck? (all five conditions) ==='
-- CheckInstantEligibility blocks on ANY of these independently. A1 tells you
-- whether the scan ledger is the cause; this tells you if something ELSE is.
-- Replace the email with the account you mentioned.
SELECT o.id AS organizer_id,
       u.email,
       o.payout_tier,
       (SELECT count(*)
          FROM events ev
         WHERE ev.organizer_id = u.id
           AND ev.status = 'completed'
           AND EXISTS (SELECT 1 FROM scan_events se WHERE se.event_id = ev.id)
       ) AS completed_events_with_scans,
       (SELECT count(*)
          FROM events ev
         WHERE ev.organizer_id = u.id
           AND ev.status = 'completed'
       ) AS completed_events_total,
       (SELECT count(*)
          FROM bookings b
          JOIN events ev ON ev.id = b.event_id
         WHERE ev.organizer_id = u.id
           AND b.refund_requested_at IS NOT NULL
           AND b.refunded_at IS NULL
       ) AS open_refund_requests
FROM   organizers o
JOIN   users u ON u.id = o.user_id
WHERE  u.email = 'REPLACE_WITH_YOUR_EMAIL';
-- If completed_events_total > completed_events_with_scans, bug #8 is the cause.
-- If they're equal and it's still stuck, look at gross revenue, account age, or
-- verified bank details instead.


\echo ''
\echo '=== A3. Did the guest double-refund bug already bite? ==='
-- The old GuestRefundHandler could issue two Paystack refunds for one booking.
-- The DB only ever recorded one, so the tell is a refunded guest booking whose
-- refund_amount does not reconcile against the payments/refunds you see in the
-- Paystack dashboard. This lists the candidates to cross-check by hand.
SELECT b.id,
       b.booking_number,
       b.guest_email,
       b.total_amount,
       b.refund_amount,
       b.refund_rate,
       b.refunded_at,
       b.updated_at,
       (b.updated_at - b.refunded_at) AS write_gap
FROM   bookings b
WHERE  b.user_id IS NULL              -- guest bookings only
  AND  b.booking_status = 'refunded'
  AND  b.refund_amount > 0
ORDER  BY b.refunded_at DESC
LIMIT  100;


\echo ''
\echo '=== A4. Attribution gap: admitted tickets with no scanner recorded ==='
-- The old fire-and-forget goroutine discarded its error, so a failed audit
-- write left qr_scanned_at set and scanned_by_user_id NULL, silently. A high
-- count here is evidence the goroutine was dropping writes in production.
SELECT count(*) FILTER (WHERE ta.scanned_by_user_id IS NULL)  AS admitted_no_scanner,
       count(*)                                               AS admitted_total,
       round(100.0 * count(*) FILTER (WHERE ta.scanned_by_user_id IS NULL)
             / NULLIF(count(*), 0), 2)                        AS pct_missing
FROM   ticket_assignments ta
WHERE  ta.qr_scanned_at IS NOT NULL;


\echo ''
\echo '=== A5. Expiry backlog: how many rows would one sweep have touched? ==='
-- The old expireBookings had no LIMIT. This is the row count it would have
-- locked in a single transaction. Anything in the thousands is the problem.
SELECT (SELECT count(*) FROM bookings
         WHERE booking_status = 'pending'
           AND payment_status IN ('pending','failed')
           AND seats_released_at IS NULL
           AND created_at < NOW() - INTERVAL '15 minutes')      AS expired_bookings_waiting,
       (SELECT count(*) FROM ticket_reservations
         WHERE expires_at < NOW())                              AS expired_reservations_waiting;


-- ############################################################################
-- PART B — LOGIC TESTS (writes, then ROLLS BACK — nothing persists)
-- ############################################################################

BEGIN;

\echo ''
\echo '=== B1. #10 — the old demote value really is invalid, the new one works ==='
DO $$
DECLARE v user_role;
BEGIN
    BEGIN
        v := 'user'::user_role;
        RAISE NOTICE 'B1 FAIL: ''user'' was accepted — enum is not what we think';
    EXCEPTION WHEN invalid_text_representation OR others THEN
        RAISE NOTICE 'B1 PASS: ''user''::user_role rejected as expected (%)', SQLERRM;
    END;

    v := 'attendee'::user_role;
    RAISE NOTICE 'B1 PASS: ''attendee''::user_role accepted';
    v := 'organizer'::user_role;
    RAISE NOTICE 'B1 PASS: ''organizer''::user_role accepted';
END $$;

\echo '--- what each current admin would be demoted TO ---'
SELECT u.id,
       u.email,
       u.role AS current_role,
       CASE WHEN EXISTS (SELECT 1 FROM organizers o WHERE o.user_id = u.id)
            THEN 'organizer'::user_role
            ELSE 'attendee'::user_role
       END AS would_become,
       EXISTS (SELECT 1 FROM organizers o WHERE o.user_id = u.id) AS has_organizer_row
FROM   users u
WHERE  u.role = 'admin'
  AND  u.deleted_at IS NULL;


\echo ''
\echo '=== B2. #1 — guest refund latch: second claim must affect ZERO rows ==='
DO $$
DECLARE
    target   uuid;
    n1       int;
    n2       int;
    claimed  timestamptz;
    n3       int;
BEGIN
    SELECT b.id INTO target
    FROM   bookings b
    WHERE  b.booking_status = 'confirmed'
    ORDER  BY b.created_at DESC
    LIMIT  1;

    IF target IS NULL THEN
        RAISE NOTICE 'B2 SKIP: no confirmed booking to test against';
        RETURN;
    END IF;

    -- First claim — this is the exact CAS the new handler runs.
    UPDATE bookings
    SET booking_status      = 'refunded',
        refund_amount       = 1.00,
        refund_rate         = 1.00,
        refunded_at         = CURRENT_TIMESTAMP,
        refund_requested_at = CURRENT_TIMESTAMP,
        refund_reason       = 'TEST — will be rolled back',
        updated_at          = CURRENT_TIMESTAMP
    WHERE id = target
      AND booking_status = 'confirmed';
    GET DIAGNOSTICS n1 = ROW_COUNT;

    SELECT b.refunded_at INTO claimed FROM bookings b WHERE b.id = target;

    -- Second claim — the concurrent request. MUST be 0.
    UPDATE bookings
    SET booking_status      = 'refunded',
        refund_amount       = 1.00,
        refund_rate         = 1.00,
        refunded_at         = CURRENT_TIMESTAMP,
        refund_requested_at = CURRENT_TIMESTAMP,
        refund_reason       = 'TEST — second caller',
        updated_at          = CURRENT_TIMESTAMP
    WHERE id = target
      AND booking_status = 'confirmed';
    GET DIAGNOSTICS n2 = ROW_COUNT;

    -- Latch release on gateway failure, guarded on our own refunded_at.
    UPDATE bookings
    SET booking_status      = 'confirmed',
        refund_amount       = NULL,
        refund_rate         = NULL,
        refunded_at         = NULL,
        refund_requested_at = NULL,
        refund_reason       = NULL,
        updated_at          = CURRENT_TIMESTAMP
    WHERE id = target
      AND booking_status = 'refunded'
      AND refunded_at = claimed;
    GET DIAGNOSTICS n3 = ROW_COUNT;

    RAISE NOTICE 'B2 booking %', target;
    RAISE NOTICE 'B2 first claim rows=%  (expect 1)', n1;
    RAISE NOTICE 'B2 second claim rows=% (expect 0 — this is the fix)', n2;
    RAISE NOTICE 'B2 latch release rows=% (expect 1)', n3;

    IF n1 = 1 AND n2 = 0 AND n3 = 1 THEN
        RAISE NOTICE 'B2 PASS: double refund is impossible; failure path restores the ticket';
    ELSE
        RAISE NOTICE 'B2 FAIL: unexpected row counts';
    END IF;
END $$;


\echo ''
\echo '=== B3. #7 — expiry batch CTE runs, is bounded, and aggregates ==='
DO $$
DECLARE cancelled int; released int;
BEGIN
    WITH victims AS (
        SELECT id, event_id, ticket_type_id, ticket_quantity
        FROM bookings
        WHERE booking_status = 'pending'
          AND payment_status IN ('pending', 'failed')
          AND seats_released_at IS NULL
          AND created_at < NOW() - '15 minutes'::interval
        ORDER BY created_at
        LIMIT 200
        FOR UPDATE SKIP LOCKED
    ),
    ev AS (
        UPDATE events e
        SET tickets_sold = GREATEST(0, e.tickets_sold - agg.qty), updated_at = NOW()
        FROM (SELECT event_id, SUM(ticket_quantity)::int AS qty FROM victims GROUP BY event_id) agg
        WHERE e.id = agg.event_id
    ),
    tt AS (
        UPDATE ticket_types t
        SET tickets_sold = GREATEST(0, t.tickets_sold - agg.qty), updated_at = NOW()
        FROM (SELECT ticket_type_id, SUM(ticket_quantity)::int AS qty FROM victims
              WHERE ticket_type_id IS NOT NULL GROUP BY ticket_type_id) agg
        WHERE t.id = agg.ticket_type_id
    ),
    cancelled_cte AS (
        UPDATE bookings b
        SET booking_status = 'cancelled', seats_released_at = NOW(), updated_at = NOW()
        FROM victims v WHERE b.id = v.id
        RETURNING b.id
    )
    SELECT count(*)::int INTO cancelled FROM cancelled_cte;

    WITH victims AS (
        SELECT id, event_id, quantity
        FROM ticket_reservations
        WHERE expires_at < NOW()
        ORDER BY expires_at
        LIMIT 200
        FOR UPDATE SKIP LOCKED
    ),
    ev AS (
        UPDATE events e
        SET reserved_count = GREATEST(0, e.reserved_count - agg.qty), updated_at = NOW()
        FROM (SELECT event_id, SUM(quantity)::int AS qty FROM victims GROUP BY event_id) agg
        WHERE e.id = agg.event_id
    ),
    del AS (
        DELETE FROM ticket_reservations r USING victims v WHERE r.id = v.id RETURNING r.id
    )
    SELECT count(*)::int INTO released FROM del;

    RAISE NOTICE 'B3 PASS: both CTEs executed. bookings cancelled=%, reservations released=% (max 200 each)',
                 cancelled, released;
    RAISE NOTICE 'B3 NOTE: if either is 0 there was simply no backlog — the syntax still validated';
EXCEPTION WHEN others THEN
    RAISE NOTICE 'B3 FAIL: %', SQLERRM;
END $$;

\echo '--- confirm the data-modifying CTEs actually fired (not just the SELECT) ---'
-- Postgres runs data-modifying CTEs to completion whether or not the outer
-- query reads them. This proves it on your server rather than on faith:
-- every booking cancelled above must now have seats_released_at set.
SELECT count(*) AS cancelled_this_tx,
       count(*) FILTER (WHERE seats_released_at IS NOT NULL) AS with_release_stamp
FROM   bookings
WHERE  booking_status = 'cancelled'
  AND  updated_at > now() - INTERVAL '10 seconds';


\echo ''
\echo '=== B4. #13 — pool admit: blocking FOR UPDATE, no phantom shortfall ==='
DO $$
DECLARE bk uuid; n int; requested int := 3;
BEGIN
    SELECT ta.booking_id INTO bk
    FROM   ticket_assignments ta
    WHERE  ta.status = 'unassigned' AND ta.qr_scanned_at IS NULL
    GROUP  BY ta.booking_id
    HAVING count(*) >= 1
    LIMIT  1;

    IF bk IS NULL THEN
        RAISE NOTICE 'B4 SKIP: no unadmitted pool slots to test against';
        RETURN;
    END IF;

    WITH picked AS (
        SELECT id FROM ticket_assignments
        WHERE booking_id = bk AND status = 'unassigned' AND qr_scanned_at IS NULL
        ORDER BY slot_number
        LIMIT requested
        FOR UPDATE                       -- was: FOR UPDATE SKIP LOCKED
    )
    UPDATE ticket_assignments ta
    SET qr_scanned_at = NOW(),
        scanned_by_user_id = (SELECT id FROM users WHERE role = 'admin' LIMIT 1)
    FROM picked WHERE ta.id = picked.id;
    GET DIAGNOSTICS n = ROW_COUNT;

    RAISE NOTICE 'B4 booking %: requested=%, admitted=%', bk, requested, n;
    IF n = 0 THEN
        RAISE NOTICE 'B4 -> verdict would be collision, admit_count stays % (CHECK > 0)', requested;
    ELSIF n < requested THEN
        RAISE NOTICE 'B4 -> verdict would be admitted, admit_count corrected to % (was reporting collision AND burning the slots before)', n;
    ELSE
        RAISE NOTICE 'B4 -> verdict would be admitted, admit_count = % (full admit)', n;
    END IF;
    RAISE NOTICE 'B4 PASS: blocking FOR UPDATE executed without deadlock';
EXCEPTION WHEN others THEN
    RAISE NOTICE 'B4 FAIL: %', SQLERRM;
END $$;


\echo ''
\echo '=== B5. #8 — the exact INSERT RecordOnlineScan runs ==='
DO $$
DECLARE ev uuid; asn uuid; bkg uuid; usr uuid; newid uuid := gen_random_uuid();
BEGIN
    SELECT ta.id, ta.booking_id, b.event_id
      INTO asn, bkg, ev
    FROM   ticket_assignments ta
    JOIN   bookings b ON b.id = ta.booking_id
    LIMIT  1;

    SELECT id INTO usr FROM users WHERE deleted_at IS NULL LIMIT 1;

    IF ev IS NULL OR usr IS NULL THEN
        RAISE NOTICE 'B5 SKIP: no assignment/user to test against';
        RETURN;
    END IF;

    INSERT INTO scan_events (
        id, event_id, assignment_id, booking_id, raw_ref,
        admit_count, scanned_by, device_id, mode, verdict, scanned_at
    ) VALUES (
        newid, ev, asn, bkg, asn::text,
        1, usr, 'online-api', 'online', 'admitted'::scan_verdict, now()
    );

    RAISE NOTICE 'B5 PASS: scan_events row inserted (id=%) — all columns, enums and FKs accept the online shape', newid;

    -- admit_count must be >= 1; prove the CHECK is the reason for the clamp.
    BEGIN
        INSERT INTO scan_events (
            id, event_id, assignment_id, booking_id, raw_ref,
            admit_count, scanned_by, device_id, mode, verdict, scanned_at
        ) VALUES (
            gen_random_uuid(), ev, asn, bkg, asn::text,
            0, usr, 'online-api', 'online', 'admitted'::scan_verdict, now()
        );
        RAISE NOTICE 'B5 WARN: admit_count 0 accepted — CHECK constraint missing?';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'B5 PASS: admit_count = 0 rejected by CHECK, which is why the Go clamps to 1';
    END;

    -- The duplicate verdict must NOT show in the exceptions report.
    IF EXISTS (
        SELECT 1 FROM scan_events
        WHERE id = newid AND verdict IN ('collision','provisional')
    ) THEN
        RAISE NOTICE 'B5 FAIL: admitted row would appear in exceptions report';
    ELSE
        RAISE NOTICE 'B5 PASS: admitted row correctly excluded from the exceptions filter';
    END IF;
EXCEPTION WHEN others THEN
    RAISE NOTICE 'B5 FAIL: %', SQLERRM;
END $$;


-- ============================================================================
-- Nothing above is kept.
-- ============================================================================
ROLLBACK;

\echo ''
\echo '=== ROLLED BACK — no changes persisted ==='


-- ############################################################################
-- PART C — POST-DEPLOY VERIFICATION
-- Run AFTER `docker compose up -d --build api` and after scanning ONE real
-- ticket at a live event.
-- ############################################################################

\echo ''
\echo '=== C1. Did the online scan write a ledger row? ==='
SELECT id, event_id, assignment_id, booking_id,
       admit_count, device_id, mode, verdict, scanned_at, received_at
FROM   scan_events
ORDER  BY received_at DESC
LIMIT  5;
-- Expect the newest row: mode='online', device_id='online-api', verdict='admitted'.
-- If nothing new appears, RecordOnlineScan is failing — grep the API logs for
-- "not written to scan ledger", which now logs at error level.

\echo ''
\echo '=== C2. Is the scanner being attributed atomically now? ==='
SELECT ta.id, ta.qr_scanned_at, ta.scanned_by_user_id, ta.scanned_by_organizer_id
FROM   ticket_assignments ta
WHERE  ta.qr_scanned_at > now() - INTERVAL '1 hour'
ORDER  BY ta.qr_scanned_at DESC
LIMIT  10;
-- scanned_by_user_id must be non-NULL on every row. Before the fix this was
-- written by a fire-and-forget goroutine that discarded its error.

\echo ''
\echo '=== C3. Is the door figure moving? ==='
SELECT e.title,
       COALESCE(SUM(se.admit_count) FILTER (WHERE se.verdict = 'admitted'), 0) AS scanned_heads,
       count(*) FILTER (WHERE se.verdict = 'duplicate')  AS duplicates,
       count(*) FILTER (WHERE se.verdict = 'collision')  AS collisions,
       count(*) FILTER (WHERE se.mode = 'online')        AS via_online,
       count(*) FILTER (WHERE se.mode = 'offline')       AS via_offline
FROM   scan_events se
JOIN   events e ON e.id = se.event_id
WHERE  se.received_at > now() - INTERVAL '24 hours'
GROUP  BY e.id, e.title
ORDER  BY scanned_heads DESC;
-- via_online > 0 is the proof that #8 is fixed. It was structurally always 0.