-- fix_legacy_buyer_claimed_slots.sql
--
-- Before the pool model, slot 1 of every booking was auto-claimed to the buyer,
-- so old bookings show the BUYER's own email in the "Assigned" list ("Claimed ·
-- has their ticket") and the group pass under-counts by one (e.g. 5 of 6).
--
-- In the pool model the buyer holds NO separate claimed ticket — they're just one
-- of the admissions in the pool. This returns those buyer-self-claimed slots to
-- the pool: the "assigned to me" row disappears and the admission count corrects
-- itself (5 -> 6). Only un-scanned slots are touched, so nobody already admitted
-- is affected.
--
-- New bookings already start fully unassigned, so this is a one-time backfill.
--
-- PowerShell:
--   Get-Content fix_legacy_buyer_claimed_slots.sql | docker compose exec -T db psql -U devuser -d events_dev

-- ── (A) Preview: which slots will be returned to the pool ────────────────────
SELECT b.booking_number,
       ta.slot_number,
       ta.assignee_email AS was_claimed_by
FROM   ticket_assignments ta
JOIN   bookings b ON b.id = ta.booking_id
LEFT   JOIN users u ON u.id = b.user_id
WHERE  ta.status = 'claimed'
  AND  ta.qr_scanned_at IS NULL
  AND  ta.assignee_email IS NOT NULL
  AND  lower(ta.assignee_email) = lower(COALESCE(u.email, b.attendee_email))
ORDER  BY b.booking_number, ta.slot_number;

-- ── (B) Return them to the pool ──────────────────────────────────────────────
UPDATE ticket_assignments ta
SET    status        = 'unassigned',
       assignee_name = NULL,
       assignee_email = NULL,
       claimed_at    = NULL,
       claim_token   = NULL
FROM   bookings b
LEFT   JOIN users u ON u.id = b.user_id
WHERE  ta.booking_id = b.id
  AND  ta.status = 'claimed'
  AND  ta.qr_scanned_at IS NULL
  AND  ta.assignee_email IS NOT NULL
  AND  lower(ta.assignee_email) = lower(COALESCE(u.email, b.attendee_email));

-- ── (C) Confirm one booking (edit the number to your test booking) ───────────
SELECT slot_number, status, assignee_email, qr_scanned_at
FROM   ticket_assignments
WHERE  booking_id = (SELECT id FROM bookings WHERE booking_number = 'BK-20260622-2AEFB5')
ORDER  BY slot_number;