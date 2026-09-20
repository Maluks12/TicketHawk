-- ═══════════════════════════════════════════════════════════════════════════
-- TicketHawk — push notification & email test harness
--
-- ⚠️  READ THIS FIRST — IT IS THE WHOLE POINT ⚠️
--
--   INSERT INTO notifications (...) DOES NOT SEND A PUSH.
--
-- There is no database trigger. Push dispatch lives in Go, inside
-- repository.CreateNotification, which fires the dispatcher hook AFTER its
-- INSERT commits. A row you insert with psql appears in the in-app feed and
-- nothing else happens — no buzz, no Expo call, no log line.
--
-- So this script does NOT insert notifications. It sets up the PRECONDITIONS
-- the background workers look for, and you then restart the API. Three of the
-- workers run a cycle immediately on startup, so the restart is the trigger.
--
-- Run order:
--   1. Section 0  — confirm your device is actually registered
--   2. curl the push-test endpoint (outside this file — see SECTION 0)
--   3. Section 1  — set your test user id
--   4. Sections 2-5 — set up worker preconditions
--   5. Restart the API container
--   6. Section 6  — verify what fired
--   7. Section 7  — reset and repeat
--
--   Section 9 (event details changed — push AND email) runs on its own
--   clock: the edit is the trigger, no restart needed. Do it any time after
--   step 3; it has its own checks and reset.
--
-- Replace <YOUR_EMAIL> throughout, or set it once in Section 1.
-- ═══════════════════════════════════════════════════════════════════════════


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 0 — Is the device even registered?
--
-- Do this before anything else. If this returns no rows, nothing below can
-- possibly buzz your phone, and you'll waste an hour debugging workers when
-- the problem is that permission was never granted.
-- ───────────────────────────────────────────────────────────────────────────

SELECT u.email,
       pt.platform,
       pt.app,
       pt.device_name,
       pt.created_at,
       pt.last_used_at,
       pt.failure_count
FROM   push_tokens pt
JOIN   users u ON u.id = pt.user_id
ORDER  BY pt.created_at DESC;

-- No rows? In the app: sign in → Profile → toggle Notifications off and on
-- again, and allow the permission prompt. Then re-run the query above.
--
-- Once you have a row, test the FULL pipeline with one HTTP call before
-- touching any of the SQL below:
--
--   curl -X POST https://api.ticket-hawk.com/v1/notifications/push-test \
--        -H "Authorization: Bearer <your access token>"
--
-- That writes a real notifications row → trips the dispatcher → sends a real
-- push. If that doesn't buzz, STOP. The problem is credentials or wiring, not
-- the workers, and none of the sections below will work either.


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 1 — Pin down your test user
-- ───────────────────────────────────────────────────────────────────────────

-- Grab your user id — you'll paste it into the sections below.
SELECT id, email, first_name, last_name, role
FROM   users
WHERE  email = 'malukstito@gmail.com';


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 2 — event_reminder  (7-day and 1-day)
--
-- Worker:  workers/reminder_worker.go — runs a cycle IMMEDIATELY on startup,
--          then hourly.
-- Query:   repository.GetFollowsNeedingReminder
--
-- Conditions, all required:
--   • event_follows row for your user           (this is what it iterates)
--   • events.status = 'published'
--   • reminded_7_days = FALSE  AND start_time BETWEEN now+6.5d AND now+7.5d
--     …or…
--   • reminded_1_day  = FALSE  AND start_time BETWEEN now+18h  AND now+30h
--
-- The 7-day window is only 24 hours wide, so "7 days from now" has to be
-- fairly precise — that's why the UPDATE below sets start_time explicitly
-- rather than hoping a seeded event happens to land in range.
-- ───────────────────────────────────────────────────────────────────────────

-- 2a. Pick any published event and park it exactly 7 days out.
WITH target AS (
    SELECT id FROM events WHERE status = 'published' ORDER BY created_at DESC LIMIT 1
)
UPDATE events e
SET    start_time = NOW() + INTERVAL '7 days',
       end_time   = NOW() + INTERVAL '7 days 4 hours'
FROM   target t
WHERE  e.id = t.id
RETURNING e.id, e.title, e.start_time;

-- 2b. Follow it as your test user, with both reminder flags cleared.
--     ON CONFLICT handles the case where you already follow it — the flag
--     reset is the part that matters on a repeat run.
INSERT INTO event_follows (user_id, event_id, reminded_7_days, reminded_1_day)
SELECT u.id, e.id, FALSE, FALSE
FROM   users u
CROSS  JOIN LATERAL (
    SELECT id FROM events
    WHERE  status = 'published'
      AND  start_time BETWEEN NOW() + INTERVAL '6.5 days' AND NOW() + INTERVAL '7.5 days'
    LIMIT  1
) e
WHERE  u.email = '<YOUR_EMAIL>'
ON CONFLICT (user_id, event_id)
DO UPDATE SET reminded_7_days = FALSE, reminded_1_day = FALSE;

-- 2c. For the 1-DAY reminder instead, run this rather than 2a
--     (start_time must land between now+18h and now+30h):
--
-- WITH target AS (
--     SELECT id FROM events WHERE status = 'published' ORDER BY created_at DESC LIMIT 1
-- )
-- UPDATE events e
-- SET    start_time = NOW() + INTERVAL '24 hours',
--        end_time   = NOW() + INTERVAL '28 hours'
-- FROM   target t
-- WHERE  e.id = t.id;

-- 2d. Confirm the worker will actually see it. If this returns 0 rows,
--     nothing will fire — check the window boundaries above.
SELECT f.id AS follow_id, u.email, e.title, e.start_time,
       f.reminded_7_days, f.reminded_1_day,
       CASE
         WHEN NOT f.reminded_7_days
              AND e.start_time BETWEEN NOW() + INTERVAL '6.5 days' AND NOW() + INTERVAL '7.5 days' THEN 7
         WHEN NOT f.reminded_1_day
              AND e.start_time BETWEEN NOW() + INTERVAL '18 hours' AND NOW() + INTERVAL '30 hours' THEN 1
       END AS days_until
FROM   event_follows f
JOIN   events e ON e.id = f.event_id
JOIN   users  u ON u.id = f.user_id
WHERE  u.email = '<YOUR_EMAIL>'
  AND  e.status = 'published'
  AND  ( (NOT f.reminded_7_days
          AND e.start_time BETWEEN NOW() + INTERVAL '6.5 days' AND NOW() + INTERVAL '7.5 days')
      OR (NOT f.reminded_1_day
          AND e.start_time BETWEEN NOW() + INTERVAL '18 hours' AND NOW() + INTERVAL '30 hours') );


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 3 — tickets_selling_fast  (80 % threshold)
--
-- Worker:  workers/selling_out_worker.go — runs once on startup, then every
--          15 minutes.
-- Query:   repository.GetSellOutCandidates
--
-- Conditions:
--   • events.status = 'published'
--   • max_capacity IS NOT NULL
--   • notified_80pct = FALSE
--   • tickets_sold::float / max_capacity >= 0.8
--   • tickets_sold < max_capacity   ← otherwise sold-out wins (see Section 4)
--
-- Fans out to EVERY follower of the event, so you need the follow row from
-- Section 2b (or the insert below).
-- ───────────────────────────────────────────────────────────────────────────

-- 3a. Push an event to exactly 80 % sold.
WITH target AS (
    SELECT id FROM events
    WHERE  status = 'published' AND max_capacity IS NOT NULL
    ORDER  BY created_at DESC LIMIT 1
)
UPDATE events e
SET    max_capacity      = 100,
       tickets_sold      = 85,   -- 85 % → over the 0.8 line, under capacity
       notified_80pct    = FALSE,
       notified_sold_out = FALSE
FROM   target t
WHERE  e.id = t.id
RETURNING e.id, e.title, e.tickets_sold, e.max_capacity;

-- 3b. Make sure you follow it (paste the id from 3a).
INSERT INTO event_follows (user_id, event_id)
SELECT u.id, '<EVENT_ID_FROM_3a>'::uuid
FROM   users u WHERE u.email = '<YOUR_EMAIL>'
ON CONFLICT (user_id, event_id) DO NOTHING;

-- 3c. Confirm the worker will see it.
SELECT id, title, tickets_sold, max_capacity,
       ROUND(tickets_sold::numeric / max_capacity * 100, 1) AS pct_sold,
       (NOT notified_80pct    AND tickets_sold::float / max_capacity >= 0.8) AS will_notify_80,
       (NOT notified_sold_out AND tickets_sold >= max_capacity)              AS will_notify_soldout
FROM   events
WHERE  status = 'published'
  AND  max_capacity IS NOT NULL
  AND  ( (NOT notified_80pct    AND tickets_sold::float / max_capacity >= 0.8)
      OR (NOT notified_sold_out AND tickets_sold >= max_capacity) );


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 4 — event_sold_out
--
-- Same worker. Sold-out takes PRIORITY over the 80 % alert: when an event is
-- both, the worker sends only "sold out" and silently flips notified_80pct so
-- you never get "selling fast" after "sold out". So test this on a SECOND
-- event, or you'll only ever see one of the two.
-- ───────────────────────────────────────────────────────────────────────────

WITH target AS (
    SELECT id FROM events
    WHERE  status = 'published' AND max_capacity IS NOT NULL
    ORDER  BY created_at DESC OFFSET 1 LIMIT 1   -- a DIFFERENT event to §3
)
UPDATE events e
SET    max_capacity      = 100,
       tickets_sold      = 100,  -- exactly at capacity
       notified_80pct    = FALSE,
       notified_sold_out = FALSE
FROM   target t
WHERE  e.id = t.id
RETURNING e.id, e.title, e.tickets_sold, e.max_capacity;

-- Follow it too (paste the id above).
INSERT INTO event_follows (user_id, event_id)
SELECT u.id, '<EVENT_ID_FROM_SECTION_4>'::uuid
FROM   users u WHERE u.email = '<YOUR_EMAIL>'
ON CONFLICT (user_id, event_id) DO NOTHING;


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 5 — booking_confirmed
--
-- Worker:  workers/booking_worker.go
-- Trigger: main.go calls repository.GetUnsentConfirmedBookingIDs on startup
--          and re-enqueues each one.
--
-- Conditions:
--   • booking_status = 'confirmed'
--   • tickets_sent   = FALSE
--   • created_at     > NOW() - INTERVAL '24 hours'   ← easy one to miss
--
-- ⚠️  This also SENDS THE TICKET EMAIL and regenerates QR codes. Use a booking
--     you already own on a test account, not a real customer's.
-- ───────────────────────────────────────────────────────────────────────────

-- 5a. Find one of your own confirmed bookings.
SELECT b.id, b.booking_number, b.booking_status, b.tickets_sent,
       b.created_at, e.title
FROM   bookings b
JOIN   users  u ON u.id = b.user_id
JOIN   events e ON e.id = b.event_id
WHERE  u.email = '<YOUR_EMAIL>'
  AND  b.booking_status = 'confirmed'
ORDER  BY b.created_at DESC
LIMIT  5;

-- 5b. Make it eligible for re-enqueue (paste the id from 5a).
UPDATE bookings
SET    tickets_sent = FALSE,
       created_at   = NOW() - INTERVAL '1 hour'  -- inside the 24h window
WHERE  id = '<BOOKING_ID_FROM_5a>'::uuid
RETURNING id, booking_number, tickets_sent, created_at;


-- ═══════════════════════════════════════════════════════════════════════════
-- ► NOW RESTART THE API ◄
--
--   docker compose -f docker-compose.prod.yml restart api
--
-- On startup: the booking worker re-enqueues §5, the reminder worker runs its
-- first cycle (§2), and the sell-out worker runs its first cycle (§3, §4).
-- Expect four pushes within a few seconds of each other.
--
-- (Section 9's event-update test is separate: its edit is the trigger, and it
-- lands 6-8 minutes after the edit whether or not you restart.)
--
-- Watch it happen:
--   docker compose -f docker-compose.prod.yml logs -f api | grep -i "push\|reminder\|sell-out"
--
-- Log lines that mean it worked:
--   push service: started
--   reminder worker: dispatching reminders  count=1
--   push service: chunk sent  ok=1 errors=0
--
-- Log lines that mean it didn't:
--   push service: queue full            → you generated a LOT of notifications
--   push service: expo rejected the request  → EXPO_ACCESS_TOKEN wrong
--   push service: pruned unregistered devices → token dead, re-register in-app
--   (nothing at all)                    → PUSH_ENABLED=false, or no push_tokens row
-- ═══════════════════════════════════════════════════════════════════════════


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 6 — What actually fired?
--
-- Notification rows are written first and are durable, so this tells you
-- whether the WORKER ran, independently of whether the PUSH arrived. That
-- split is the useful diagnostic:
--
--   row present + no buzz   → push layer (token, credentials, permission)
--   no row at all           → worker preconditions were never met (redo §2-5)
-- ───────────────────────────────────────────────────────────────────────────

SELECT n.created_at, n.type, n.title, n.body, n.is_read, n.data
FROM   notifications n
JOIN   users u ON u.id = n.user_id
WHERE  u.email = '<YOUR_EMAIL>'
  AND  n.created_at > NOW() - INTERVAL '15 minutes'
ORDER  BY n.created_at DESC;

-- Coverage check — which types have you managed to fire?
SELECT n.type, COUNT(*) AS fired, MAX(n.created_at) AS most_recent
FROM   notifications n
JOIN   users u ON u.id = n.user_id
WHERE  u.email = '<YOUR_EMAIL>'
GROUP  BY n.type
ORDER  BY most_recent DESC;


-- ───────────────────────────────────────────────────────────────────────────
-- SECTION 7 — Reset, to run the whole thing again
--
-- Every worker flips a "sent" flag so it never repeats. Clearing them is what
-- makes this script re-runnable.
-- ───────────────────────────────────────────────────────────────────────────

-- Reminder flags
UPDATE event_follows f
SET    reminded_7_days = FALSE, reminded_1_day = FALSE
FROM   users u
WHERE  u.id = f.user_id AND u.email = '<YOUR_EMAIL>';

-- Sell-out flags (only the events you touched — do NOT blanket-reset in prod,
-- it would re-alert every follower of every event that's ever crossed 80 %)
UPDATE events
SET    notified_80pct = FALSE, notified_sold_out = FALSE
WHERE  id IN ('<EVENT_ID_FROM_3a>'::uuid, '<EVENT_ID_FROM_SECTION_4>'::uuid);

-- Clear the test notifications out of your feed
DELETE FROM notifications n
USING  users u
WHERE  u.id = n.user_id
  AND  u.email = '<YOUR_EMAIL>'
  AND  n.created_at > NOW() - INTERVAL '1 hour';


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 8 — The three types SQL can't reach
--
-- event_cancelled   Emitted from repository/events_repository.go when an
--                   organiser cancels. It runs inside the cancellation
--                   transaction alongside refund logic, so forcing it with a
--                   status UPDATE would fire the notification without the
--                   refunds — a misleading test. Cancel a test event through
--                   the organiser dashboard instead.
--
-- kyc_approved      Emitted from repository/organizer_respository.go on
--                   tier-2 verification. Trigger it by approving a pending
--                   organiser in the admin dashboard.
--
-- refund_processed  Emitted by RequestRefundHandler
--                   (handlers/booking_handler.go) when a buyer refunds their
--                   own booking, and by RefundService.markBookingRefunded
--                   (services/refund_service.go) for cancellation refunds.
--                   Both run inside the real Paystack refund, so request a
--                   refund on one of your own test bookings in the app rather
--                   than faking it here.
-- ═══════════════════════════════════════════════════════════════════════════


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 9 — event_details_changed   (in-app + push AND email)
--
-- Runs on its own clock — NOT part of the restart above.
--
-- Worker:  workers/event_details_change_worker.go — polls every 2 minutes.
--          It compares each live event's details with what ticket holders
--          were last told (event_detail_watch) and announces a change once
--          the details have been still for 5 minutes. The EDIT is the
--          trigger: expect the push 6-8 minutes after it, and the email
--          within a minute of the push.
--
-- Who is told:
--   • accounts with a CONFIRMED TICKET booking → in-app + push + email
--   • guest bookings (no account)              → email
--   • claimed ticket assignments               → email, with their own link
--   Never: the event's organiser, deleted accounts, cancelled or refunded
--   bookings, merch-only orders.
--
-- ⚠️  THREE TRAPS
--   1. The organiser is never told about their own edit. Your test account
--      must hold a ticket to an event ANOTHER account organises — edit as
--      that organiser, receive as your test account.
--   2. EVERY ticket holder of the event gets the email. Run 9b and only
--      carry on if you are the only person listed.
--   3. The event must already be watched when you edit it (9a: watched =
--      TRUE). A newly published event is baselined on the worker's first
--      look, up to 2 minutes after it goes live; an edit made before that
--      becomes part of the baseline and is never announced.
--
-- The realistic trigger is the organiser dashboard: on a live event the
-- Information editor now shows a "ticket holders are told what changed"
-- notice — change something there and save. 9c makes the same kind of edit
-- in SQL.
-- ═══════════════════════════════════════════════════════════════════════════

-- 9a. Events you hold a ticket for. Pick a row with you_are_organiser = FALSE
--     and watched = TRUE. last_notice must be empty or over an hour old (one
--     notice per event per hour; 15 minutes within a day of the start) — 9h
--     clears it.
SELECT DISTINCT
       e.id, e.title, e.status, e.start_time,
       (e.organizer_id = u.id)  AS you_are_organiser,
       (w.event_id IS NOT NULL) AS watched,
       w.notified_at            AS last_notice
FROM   bookings b
JOIN   users  u ON u.id = b.user_id
JOIN   events e ON e.id = b.event_id
LEFT   JOIN event_detail_watch w ON w.event_id = e.id
WHERE  u.email = '<YOUR_EMAIL>'
  AND  b.booking_status = 'confirmed'
  AND  b.booking_type   = 'ticket'
  AND  e.status IN ('published', 'published_private')
  AND  e.end_time >= NOW()
ORDER  BY e.start_time;

-- 9b. Who will be emailed — the worker's own audience rules, one row per
--     address (an account's link wins over a guest or claim link for the
--     same address). The 'account' rows also get the in-app notification
--     and push.
--     ⚠️  If anyone but you is listed, STOP and pick another event.
WITH ev AS (SELECT '<EVENT_ID_FROM_9a>'::uuid AS id)
SELECT DISTINCT ON (lower(r.email))
       r.email, r.link_kind, r.name
FROM (
    SELECT btrim(u.email) AS email, 'account' AS link_kind, 1 AS priority,
           concat_ws(' ', u.first_name, u.last_name) AS name
    FROM   bookings b
    JOIN   ev         ON ev.id = b.event_id
    JOIN   events e   ON e.id  = b.event_id
    JOIN   users  u   ON u.id  = b.user_id
    WHERE  b.booking_status = 'confirmed'
      AND  b.booking_type   = 'ticket'
      AND  u.deleted_at IS NULL
      AND  b.user_id <> e.organizer_id

    UNION ALL

    SELECT btrim(b.attendee_email), 'guest', 2,
           concat_ws(' ', b.attendee_first_name, b.attendee_last_name)
    FROM   bookings b
    JOIN   ev ON ev.id = b.event_id
    WHERE  b.booking_status = 'confirmed'
      AND  b.booking_type   = 'ticket'
      AND  b.user_id IS NULL
      AND  COALESCE(btrim(b.attendee_email), '') <> ''

    UNION ALL

    SELECT btrim(ta.assignee_email), 'claimant', 3, ta.assignee_name
    FROM   ticket_assignments ta
    JOIN   bookings b ON b.id  = ta.booking_id
    JOIN   ev         ON ev.id = b.event_id
    WHERE  b.booking_status = 'confirmed'
      AND  b.booking_type   = 'ticket'
      AND  ta.status        = 'claimed'
      AND  COALESCE(btrim(ta.assignee_email), '') <> ''
) r
ORDER  BY lower(r.email), r.priority;

-- 9c. The edit. Note the current Parking text first if you'll want it back:
SELECT info_key, value
FROM   event_info_items
WHERE  event_id = '<EVENT_ID_FROM_9a>'::uuid
  AND  info_key = 'parking';

--     Then change it. An upsert, so it works whether or not the event has a
--     Parking note. The clock time makes every run a real change — saving
--     identical text is correctly ignored, and so is a case- or spacing-only
--     edit.
INSERT INTO event_info_items (event_id, info_key, value)
VALUES ('<EVENT_ID_FROM_9a>'::uuid, 'parking',
        'North lot (test ' || to_char(NOW() AT TIME ZONE 'Africa/Johannesburg', 'HH24:MI') || ')')
ON CONFLICT (event_id, info_key) DO UPDATE SET value = EXCLUDED.value
RETURNING info_key, value;

-- 9d. (Optional) Also move the start and end 30 minutes later. Run it within
--     5 minutes of 9c and both arrive as ONE notice — the settle window
--     batching edits is part of what you're testing. A time change makes the
--     push title "Date & time changed" and puts the red panel in the email.
UPDATE events
SET    start_time = start_time + INTERVAL '30 minutes',
       end_time   = end_time   + INTERVAL '30 minutes'
WHERE  id = '<EVENT_ID_FROM_9a>'::uuid
RETURNING id, title, start_time, end_time;
--     Direct SQL skips the API's cache busting, so the app's event page may
--     show the old time until its cache expires. The notification and email
--     are built from the database, so they're right regardless.

-- 9e. Watch the worker pick it up. Re-run every minute or so:
--       change_pending = TRUE                 → seen; goes out once
--                                               stable_for passes 5 minutes
--       notified_at just set, pending FALSE   → announced
SELECT w.event_id,
       (w.pending_snapshot IS NOT NULL) AS change_pending,
       NOW() - w.pending_since          AS stable_for,
       w.notified_at,
       w.announced_at
FROM   event_detail_watch w
WHERE  w.event_id = '<EVENT_ID_FROM_9a>'::uuid;

-- 9f. (Optional) Skip the 5-minute wait once 9e shows change_pending = TRUE.
--     It then goes out on the worker's next pass, within 2 minutes.
-- UPDATE event_detail_watch
-- SET    pending_since = NOW() - INTERVAL '6 minutes'
-- WHERE  event_id = '<EVENT_ID_FROM_9a>'::uuid
--   AND  pending_since IS NOT NULL;

-- Watch it:
--   docker compose -f docker-compose.prod.yml logs -f api | grep -iE "event details|push service"
--
-- Log lines that mean it worked:
--   event details worker: change announced   (with in_app=1 emails_queued=1)
--   push service: chunk sent  ok=1 errors=0
--
-- What should arrive:
--   Push   "Event update" — "<title>: Parking now North lot (test 14:05)."
--          ("Parking added: …" if the event had no Parking note.)
--          With 9d: "Date & time changed" — "<title>: Date & time now
--          Sat 14 Nov, 19:30; Parking now North lot (test 14:05)."
--   Tap    opens the event page.
--   Email  subject "Update to <title>" (with 9d: "Date & time changed:
--          <title>"), a "What changed" panel, the current details and a
--          "View my booking" button.

-- 9g. What fired. The in-app row proves the worker announced; the notice is
--     the record of what was said; the email rows show delivery.
SELECT n.created_at, n.title, n.body, n.data->>'event_id' AS event_id
FROM   notifications n
JOIN   users u ON u.id = n.user_id
WHERE  u.email = '<YOUR_EMAIL>'
  AND  n.type  = 'event_details_changed'
ORDER  BY n.created_at DESC
LIMIT  5;

SELECT created_at, event_title, summary, in_app_count, email_count
FROM   event_update_notices
WHERE  event_id = '<EVENT_ID_FROM_9a>'::uuid
ORDER  BY created_at DESC
LIMIT  5;
--   no row                   → not announced yet (see 9e), or nobody held a
--                              ticket, in which case it is accepted silently
--   in_app_count = 0 AND
--   email_count  = 0         → the only ticket holder is the organiser (trap 1)

SELECT q.created_at, q.email, q.link_kind, q.status, q.attempts,
       q.last_error, q.sent_at
FROM   event_update_emails q
WHERE  q.event_id = '<EVENT_ID_FROM_9a>'::uuid
ORDER  BY q.created_at DESC;
--   pending / sending  → the email drain runs every 20 seconds; give it a minute
--   sent               → SES accepted it; check the inbox, then spam
--   failed             → last_error says why (SES rejection, or retries used up)
--   expired            → the event ended before the email could go out

-- 9h. Reset.
--     Clear the one-notice-per-hour gap so the next edit goes out as soon as
--     it settles:
UPDATE event_detail_watch
SET    notified_at = NULL
WHERE  event_id = '<EVENT_ID_FROM_9a>'::uuid;

--     Clear the test notices (their email rows go with them) and the in-app
--     rows out of your feed:
DELETE FROM event_update_notices
WHERE  event_id = '<EVENT_ID_FROM_9a>'::uuid
  AND  created_at > NOW() - INTERVAL '1 hour';

DELETE FROM notifications n
USING  users u
WHERE  u.id = n.user_id
  AND  u.email = '<YOUR_EMAIL>'
  AND  n.type  = 'event_details_changed'
  AND  n.created_at > NOW() - INTERVAL '1 hour';

--     Putting the event back. Reverting is itself a change, and it WILL be
--     announced 6-8 minutes later. To revert silently, undo the edit FIRST,
--     then delete the watch row: on its next pass the worker re-baselines the
--     event from its current details without telling anyone. Keep that order
--     — delete first and the worker may baseline the un-reverted details,
--     then announce the revert.
--
-- UPDATE event_info_items
-- SET    value = '<PARKING_TEXT_FROM_9c>'
-- WHERE  event_id = '<EVENT_ID_FROM_9a>'::uuid AND info_key = 'parking';
--   …or, if 9c showed no Parking note:
-- DELETE FROM event_info_items
-- WHERE  event_id = '<EVENT_ID_FROM_9a>'::uuid AND info_key = 'parking';
--
-- UPDATE events                                   -- only if you ran 9d
-- SET    start_time = start_time - INTERVAL '30 minutes',
--        end_time   = end_time   - INTERVAL '30 minutes'
-- WHERE  id = '<EVENT_ID_FROM_9a>'::uuid;
--
-- DELETE FROM event_detail_watch WHERE event_id = '<EVENT_ID_FROM_9a>'::uuid;