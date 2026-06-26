-- enable_scan_test_2.sql
-- Makes a DIFFERENT event scannable right now (so you can test the pool scan on a
-- clean event). Same idea as enable_scan_test.sql, just a different target.
--
-- Default target = event ...118. If you'd rather pick by name, run the SELECT in
-- section (A) first, copy the id you want into EVENT_ID, then run the rest.
--
-- PowerShell:
--   Get-Content enable_scan_test_2.sql | docker compose exec -T db psql -U devuser -d events_dev

-- ── (A) OPTIONAL — list upcoming events to pick a different one ──────────────
--    Comment this out once you've chosen; it just prints a menu.
SELECT id, title, start_time::timestamptz(0) AS starts
FROM   events
WHERE  start_time > NOW()
ORDER  BY start_time
LIMIT  15;

-- ── (B) Pick the event + (optionally) your account. Edit these. ──────────────
\set EVENT_ID '''e2000000-0000-0000-0000-000000000118'''

-- ── (C) Move the event into the live admission window. ──────────────────────
UPDATE events
SET    start_time = NOW() - INTERVAL '30 minutes',
       end_time   = NOW() + INTERVAL '3 hours',
       status     = 'published'
WHERE  id = :EVENT_ID::uuid;

-- ── (D) OPTIONAL — make yourself the organizer so you can scan it. ──────────
--    Uncomment and set your email (find it via the confirm SELECT below first).
-- UPDATE events
-- SET    organizer_id = (SELECT id FROM users WHERE email = 'YOUR_EMAIL_HERE')
-- WHERE  id = :EVENT_ID::uuid;

-- ── (E) Confirm: window open + who must scan it. ────────────────────────────
SELECT e.title,
       e.start_time::timestamptz(0) AS starts,
       e.end_time::timestamptz(0)   AS ends,
       (NOW() BETWEEN e.start_time - INTERVAL '2 hours'
                  AND e.end_time   + INTERVAL '2 hours') AS scannable_now,
       u.email                      AS organizer_email_must_scan_as
FROM   events e
JOIN   users  u ON u.id = e.organizer_id
WHERE  e.id = :EVENT_ID::uuid;