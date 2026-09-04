-- ============================================================================
-- TicketHawk — verification script for the scan-ledger / refund / expiry fixes
--
-- v2. Corrections from the first run:
--   * A3 referenced bookings.guest_email, which does not exist. The columns
--     are attendee_email and is_guest.
--   * A2 required hand-editing an email placeholder and silently returned
--     zero rows when it wasn't replaced. It now lists every organizer.
--   * A2 was missing two of the five eligibility conditions (verified bank,
--     account age) and did not show the thresholds being compared against.
--   * B3 and B4 both passed on empty data and so tested almost nothing. They
--     now SEED the case they are meant to exercise.
--   * Added A0 (has anything ever been scanned) and A6 (promotion is
--     admin-initiated only — nothing calls SetOrganizerTier automatically).
--
-- PART A is read-only. Safe on production, any time.
-- PART B writes and then ROLLS BACK. Nothing persists. It briefly takes row
--        locks (FOR UPDATE), so prefer staging or a quiet window.
-- PART C is read-only. Run AFTER deploying and scanning one real ticket.
-- ============================================================================


-- ############################################################################
-- PART A — READ-ONLY DIAGNOSTICS
-- ############################################################################

\echo '=== A0. Has ANY ticket ever been scanned? ==='
-- Run this first. If assignments_admitted is 0, A1 and the scan-ledger
-- backfill are both moot: bug #8 cannot have lost data that was never
-- written. It also means no organizer can satisfy the instant-tier event
-- count, because that count requires EXISTS (SELECT 1 FROM scan_events ...)
-- for each event.
SELECT (SELECT count(*) FROM ticket_assignments WHERE qr_scanned_at IS NOT NULL) AS assignments_admitted,
       (SELECT count(*) FROM scan_events)                                        AS ledger_rows_total,
       (SELECT count(*) FROM scan_events WHERE mode = 'online')                  AS ledger_rows_online,
       (SELECT count(*) FROM scan_events WHERE mode = 'offline')                 AS ledger_rows_offline;


\echo ''
\echo '=== A1. Scan-ledger gap: events that happened but are invisible ==='
-- Only meaningful if A0 shows assignments_admitted > 0.
-- admitted_assignments > 0 AND ledger_rows = 0  =>  invisible to both the
-- analytics door figure and CheckInstantEligibility.
SELECT e.id,
       e.title,
       e.status,
       e.start_time,
       count(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL)           AS admitted_assignments,
       (SELECT count(*) FROM scan_events se WHERE se.event_id = e.id) AS ledger_rows,
       CASE
         WHEN count(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL) > 0
          AND (SELECT count(*) FROM scan_events se WHERE se.event_id = e.id) = 0
         THEN 'INVISIBLE — needs backfill'
         ELSE 'ok'
       END AS verdict
FROM   events e
JOIN   bookings b            ON b.event_id    = e.id
JOIN   ticket_assignments ta ON ta.booking_id = b.id
GROUP  BY e.id, e.title, e.status, e.start_time
HAVING count(*) FILTER (WHERE ta.qr_scanned_at IS NOT NULL) > 0
ORDER  BY e.start_time DESC;


\echo ''
\echo '=== A2. Instant-tier eligibility — every organizer, all five conditions ==='
-- Mirrors CheckInstantEligibility exactly. Thresholds from models/payout.go:
--   qualifying_events  >= 2        (InstantTierMinCompletedEvents)
--   gross              >= 5000.00  (InstantTierMinGrossZAR)
--   account_age_days   >= 30       (InstantTierMinAccountAgeDays)
--   has_verified_bank  = true
--   open_refunds       = 0
--
-- qualifying_events counts ONLY events that are completed AND have at least
-- one scan_events row AND at least one confirmed booking with subtotal > 0.
-- Compare it against completed_events: a gap means the scan requirement is
-- what's holding the account back, not sales.
SELECT u.email,
       o.payout_tier,
       (SELECT count(DISTINCT ev.id) FROM events ev
         WHERE ev.organizer_id = o.user_id
           AND ev.status = 'completed'
           AND EXISTS (SELECT 1 FROM scan_events se WHERE se.event_id = ev.id)
           AND EXISTS (SELECT 1 FROM bookings pb
                        WHERE pb.event_id = ev.id
                          AND pb.booking_status = 'confirmed'
                          AND pb.subtotal > 0)
       )                                                                AS qualifying_events,
       (SELECT count(*) FROM events ev
         WHERE ev.organizer_id = o.user_id AND ev.status = 'completed')  AS completed_events,
       (SELECT COALESCE(SUM(b.subtotal), 0) FROM bookings b
          JOIN events ev ON ev.id = b.event_id
         WHERE ev.organizer_id = o.user_id
           AND b.booking_status = 'confirmed')                          AS gross,
       EXTRACT(day FROM now() - o.created_at)::int                      AS account_age_days,
       (o.paystack_subaccount_code IS NOT NULL AND o.payout_enabled)    AS has_verified_bank,
       (SELECT count(*) FROM bookings b
          JOIN events ev ON ev.id = b.event_id
         WHERE ev.organizer_id = o.user_id
           AND b.refund_requested_at IS NOT NULL
           AND b.refunded_at IS NULL)                                   AS open_refunds
FROM   organizers o
JOIN   users u ON u.id = o.user_id
ORDER  BY gross DESC;


\echo ''
\echo '=== A3. Did the guest double-refund bug already bite? ==='
-- The old GuestRefundHandler could issue two Paystack refunds for one booking.
-- The DB only ever recorded one, so the tell is a refunded guest booking whose
-- refund_amount does not reconcile against the Paystack dashboard. These are
-- the rows to cross-check by hand.
SELECT b.id,
       b.booking_number,
       b.attendee_email,
       b.total_amount,
       b.refund_amount,
       b.refund_rate,
       b.refunded_at,
       b.updated_at
FROM   bookings b
WHERE  b.is_guest = true
  AND  b.booking_status = 'refunded'
  AND  b.refund_amount > 0
ORDER  BY b.refunded_at DESC
LIMIT  100;


\echo ''
\echo '=== A4. Attribution gap: admitted tickets with no scanner recorded ==='
-- The old fire-and-forget goroutine discarded its error, so a failed audit
-- write left qr_scanned_at set and scanned_by_user_id NULL, silently.
SELECT count(*) FILTER (WHERE ta.scanned_by_user_id IS NULL)  AS admitted_no_scanner,
       count(*)                                               AS admitted_total,
       round(100.0 * count(*) FILTER (WHERE ta.scanned_by_user_id IS NULL)
             / NULLIF(count(*), 0), 2)                        AS pct_missing
FROM   ticket_assignments ta
WHERE  ta.qr_scanned_at IS NOT NULL;


\echo ''
\echo '=== A5. Expiry backlog: how many rows would one sweep have touched? ==='
-- The old expireBookings had no LIMIT. This is the row count it would have
-- locked in a single transaction.
SELECT (SELECT count(*) FROM bookings
         WHERE booking_status = 'pending'
           AND payment_status IN ('pending','failed')
           AND seats_released_at IS NULL
           AND created_at < NOW() - INTERVAL '15 minutes')  AS expired_bookings_waiting,
       (SELECT count(*) FROM ticket_reservations
         WHERE expires_at < NOW())                          AS expired_reservations_waiting;


\echo ''
\echo '=== A6. Payout tier distribution ==='
-- Promotion is ADMIN-INITIATED ONLY. SetOrganizerTier has exactly one caller,
-- AdminSetOrganizerTierHandler, and CheckInstantEligibility is only ever READ
-- — to render a badge in the admin list and a progress display on the
-- organizer's own payout screen. No worker, cron or webhook acts on it. An
-- account can satisfy all five conditions and stay on held tier indefinitely
-- until someone opens the console and clicks.
SELECT o.payout_tier,
       count(*)                                              AS organizers,
       count(*) FILTER (WHERE o.paystack_subaccount_code IS NOT NULL
                          AND o.payout_enabled)              AS with_verified_bank
FROM   organizers o
GROUP  BY o.payout_tier
ORDER  BY o.payout_tier;


-- ############################################################################
-- PART B — LOGIC TESTS (writes, then ROLLS BACK — nothing persists)
-- ############################################################################

BEGIN;

\echo ''
\echo '=== B1. #10 — the old demote value is invalid, the new one works ==='
DO $$
DECLARE v user_role;
BEGIN
    BEGIN
        v := 'user'::user_role;
        RAISE NOTICE 'B1 FAIL: ''user'' was accepted — enum is not what we think';
    EXCEPTION WHEN others THEN
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
DECLARE target uuid; n1 int; n2 int; n3 int; claimed timestamptz;
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

    UPDATE bookings
    SET booking_status = 'refunded', refund_amount = 1.00, refund_rate = 1.00,
        refunded_at = CURRENT_TIMESTAMP, refund_requested_at = CURRENT_TIMESTAMP,
        refund_reason = 'TEST — will be rolled back', updated_at = CURRENT_TIMESTAMP
    WHERE id = target AND booking_status = 'confirmed';
    GET DIAGNOSTICS n1 = ROW_COUNT;

    SELECT b.refunded_at INTO claimed FROM bookings b WHERE b.id = target;

    -- The concurrent request. MUST be 0.
    UPDATE bookings
    SET booking_status = 'refunded', refund_amount = 1.00, refund_rate = 1.00,
        refunded_at = CURRENT_TIMESTAMP, refund_requested_at = CURRENT_TIMESTAMP,
        refund_reason = 'TEST — second caller', updated_at = CURRENT_TIMESTAMP
    WHERE id = target AND booking_status = 'confirmed';
    GET DIAGNOSTICS n2 = ROW_COUNT;

    -- Latch release on gateway failure, guarded on our own refunded_at.
    UPDATE bookings
    SET booking_status = 'confirmed', refund_amount = NULL, refund_rate = NULL,
        refunded_at = NULL, refund_requested_at = NULL, refund_reason = NULL,
        updated_at = CURRENT_TIMESTAMP
    WHERE id = target AND booking_status = 'refunded' AND refunded_at = claimed;
    GET DIAGNOSTICS n3 = ROW_COUNT;

    RAISE NOTICE 'B2 booking %', target;
    RAISE NOTICE 'B2 first claim rows=%  (expect 1)', n1;
    RAISE NOTICE 'B2 second claim rows=% (expect 0 — this is the fix)', n2;
    RAISE NOTICE 'B2 latch release rows=% (expect 1)', n3;
    IF n1 = 1 AND n2 = 0 AND n3 = 1 THEN
        RAISE NOTICE 'B2 PASS: double refund impossible; failure path restores the ticket';
    ELSE
        RAISE NOTICE 'B2 FAIL: unexpected row counts';
    END IF;
END $$;


\echo ''
\echo '=== B3. #7 — expiry batch CTE: bounded, and the aggregation is correct ==='
-- The v1 run validated syntax only, because there was no backlog. This version
-- SEEDS two expired bookings on the SAME event so the per-event aggregation
-- actually executes. That was the untested part, and it is exactly what the
-- per-row loop it replaced structurally could not do: a single UPDATE cannot
-- touch the same events row twice.
DO $$
DECLARE
    ev uuid; tt uuid; b1 uuid; b2 uuid;
    sold_before int; sold_after int; cancelled int;
BEGIN
    SELECT e.id INTO ev FROM events e LIMIT 1;
    IF ev IS NULL THEN
        RAISE NOTICE 'B3 SKIP: no events to test against';
        RETURN;
    END IF;
    SELECT t.id INTO tt FROM ticket_types t WHERE t.event_id = ev LIMIT 1;

    SELECT e.tickets_sold INTO sold_before FROM events e WHERE e.id = ev;

    -- booking_number (VARCHAR(20) UNIQUE NOT NULL) and unit_price (NOT NULL,
    -- no default) must both be supplied. attendee_email is required too:
    -- chk_booking_identity (migration 000008) demands it whenever is_guest
    -- is TRUE, which these rows are because they carry no user_id.
    INSERT INTO bookings (id, event_id, ticket_type_id, ticket_quantity,
                          booking_number, unit_price, attendee_email,
                          booking_status, payment_status, seats_released_at,
                          created_at, updated_at, subtotal, total_amount, is_guest)
    VALUES (gen_random_uuid(), ev, tt, 3,
            'ZZTEST-' || substr(gen_random_uuid()::text, 1, 12), 0, 'b3test@example.invalid',
            'pending', 'pending', NULL,
            NOW() - INTERVAL '2 hours', NOW(), 0, 0, true)
    RETURNING id INTO b1;

    INSERT INTO bookings (id, event_id, ticket_type_id, ticket_quantity,
                          booking_number, unit_price, attendee_email,
                          booking_status, payment_status, seats_released_at,
                          created_at, updated_at, subtotal, total_amount, is_guest)
    VALUES (gen_random_uuid(), ev, tt, 3,
            'ZZTEST-' || substr(gen_random_uuid()::text, 1, 12), 0, 'b3test@example.invalid',
            'pending', 'failed', NULL,
            NOW() - INTERVAL '2 hours', NOW(), 0, 0, true)
    RETURNING id INTO b2;

    UPDATE events SET tickets_sold = tickets_sold + 6 WHERE id = ev;

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
    ev_cte AS (
        UPDATE events e
        SET tickets_sold = GREATEST(0, e.tickets_sold - agg.qty), updated_at = NOW()
        FROM (SELECT event_id, SUM(ticket_quantity)::int AS qty FROM victims GROUP BY event_id) agg
        WHERE e.id = agg.event_id
    ),
    tt_cte AS (
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

    SELECT e.tickets_sold INTO sold_after FROM events e WHERE e.id = ev;

    RAISE NOTICE 'B3 cancelled=% (expect >= 2)', cancelled;
    RAISE NOTICE 'B3 event tickets_sold: % -> % (seeded +6, expect back to %)',
                 sold_before + 6, sold_after, sold_before;

    IF sold_after = sold_before THEN
        RAISE NOTICE 'B3 PASS: both seeded bookings decremented the SAME event in ONE update — the aggregation works';
    ELSE
        RAISE NOTICE 'B3 FAIL: expected tickets_sold back to %, got %', sold_before, sold_after;
    END IF;

    IF (SELECT count(*) FROM bookings WHERE id IN (b1, b2) AND seats_released_at IS NOT NULL) = 2 THEN
        RAISE NOTICE 'B3 PASS: data-modifying CTEs ran to completion (release stamps set)';
    ELSE
        RAISE NOTICE 'B3 FAIL: seats_released_at not stamped';
    END IF;
EXCEPTION WHEN others THEN
    RAISE NOTICE 'B3 FAIL: %', SQLERRM;
END $$;


\echo ''
\echo '=== B4. #13 — pool admit: blocking FOR UPDATE, correct partial handling ==='
-- v1 happened to pick a booking with one free slot, so the shortfall branch
-- was barely exercised. This picks the booking with the MOST free slots and
-- deliberately requests more than exist, forcing the partial path.
DO $$
DECLARE bk uuid; avail int; n int; requested int;
BEGIN
    SELECT ta.booking_id, count(*) INTO bk, avail
    FROM   ticket_assignments ta
    WHERE  ta.status = 'unassigned' AND ta.qr_scanned_at IS NULL
    GROUP  BY ta.booking_id
    ORDER  BY count(*) DESC
    LIMIT  1;

    IF bk IS NULL THEN
        RAISE NOTICE 'B4 SKIP: no unadmitted pool slots to test against';
        RETURN;
    END IF;

    requested := avail + 2;   -- deliberately more than exist

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

    RAISE NOTICE 'B4 booking %: available=%, requested=%, admitted=%', bk, avail, requested, n;
    IF n = 0 THEN
        RAISE NOTICE 'B4 -> verdict collision, admit_count stays % (CHECK > 0)', requested;
    ELSIF n < requested THEN
        RAISE NOTICE 'B4 -> verdict admitted, admit_count corrected % -> % (old code called this a collision AND burned the slots)', requested, n;
    ELSE
        RAISE NOTICE 'B4 -> verdict admitted, admit_count = % (full admit)', n;
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
    SELECT ta.id, ta.booking_id, b.event_id INTO asn, bkg, ev
    FROM   ticket_assignments ta
    JOIN   bookings b ON b.id = ta.booking_id
    LIMIT  1;
    SELECT id INTO usr FROM users WHERE deleted_at IS NULL LIMIT 1;

    IF ev IS NULL OR usr IS NULL THEN
        RAISE NOTICE 'B5 SKIP: no assignment/user to test against';
        RETURN;
    END IF;

    INSERT INTO scan_events (id, event_id, assignment_id, booking_id, raw_ref,
                             admit_count, scanned_by, device_id, mode, verdict, scanned_at)
    VALUES (newid, ev, asn, bkg, asn::text,
            1, usr, 'online-api', 'online', 'admitted'::scan_verdict, now());
    RAISE NOTICE 'B5 PASS: scan_events row inserted (id=%) — columns, enums and FKs all accept the online shape', newid;

    BEGIN
        INSERT INTO scan_events (id, event_id, assignment_id, booking_id, raw_ref,
                                 admit_count, scanned_by, device_id, mode, verdict, scanned_at)
        VALUES (gen_random_uuid(), ev, asn, bkg, asn::text,
                0, usr, 'online-api', 'online', 'admitted'::scan_verdict, now());
        RAISE NOTICE 'B5 WARN: admit_count 0 accepted — CHECK constraint missing?';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'B5 PASS: admit_count = 0 rejected by CHECK, which is why the Go clamps to 1';
    END;

    IF EXISTS (SELECT 1 FROM scan_events WHERE id = newid AND verdict IN ('collision','provisional')) THEN
        RAISE NOTICE 'B5 FAIL: admitted row would appear in the exceptions report';
    ELSE
        RAISE NOTICE 'B5 PASS: admitted row correctly excluded from the exceptions filter';
    END IF;

    -- Prove the analytics door query would now count it.
    RAISE NOTICE 'B5 analytics door figure for this event would now be: %',
        (SELECT COALESCE(SUM(admit_count), 0) FROM scan_events
          WHERE event_id = ev AND verdict = 'admitted');
EXCEPTION WHEN others THEN
    RAISE NOTICE 'B5 FAIL: %', SQLERRM;
END $$;


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
-- Expect newest: mode='online', device_id='online-api', verdict='admitted'.
-- If nothing appears, grep the API logs for "not written to scan ledger" —
-- that path now logs at error level instead of failing silently.

\echo ''
\echo '=== C2. Is the scanner attributed atomically now? ==='
SELECT ta.id, ta.qr_scanned_at, ta.scanned_by_user_id, ta.scanned_by_organizer_id
FROM   ticket_assignments ta
WHERE  ta.qr_scanned_at > now() - INTERVAL '1 hour'
ORDER  BY ta.qr_scanned_at DESC
LIMIT  10;
-- scanned_by_user_id must be non-NULL on every row.

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
-- via_online > 0 is the proof #8 is fixed. It was structurally always 0.