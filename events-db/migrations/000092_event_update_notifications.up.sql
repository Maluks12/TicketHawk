-- ============================================================================
-- 000092: Event detail change notifications.
--
-- THE GAP THIS CLOSES
--
-- A live event's details change by several routes:
--
--   organiser    PUT /organiser/events/:id/info — the Information block
--                (parking, doors, dress code, prohibited items, ...), applied
--                immediately and deliberately so: "the north gate is closed"
--                cannot wait in an admin queue.
--   edit request title, description and times, applied by the executor once
--                an admin approves. The console can also move venue and date.
--   takedown     an event taken down for a fix is a draft with buyers still
--                attached, edited freely, then republished.
--
-- None of them told the people holding tickets. Someone who bought against
-- "doors 18:00, park at the north gate" found out about "doors 19:00, north
-- gate closed" by arriving.
--
-- Same answer as 000082 gave the line-up: do not block the change, tell the
-- people with a stake. This time by email as well, because 000082's open gap —
-- guest buyers have no account, so no inbox and no push — matters far more for
-- a moved venue than for a changed support act.
--
-- WHY A SIDE TABLE, NOT COLUMNS ON events (unlike 000078 and 000082)
--
-- events carries a BEFORE UPDATE trigger (update_events_updated_at, 000004)
-- that stamps updated_at on EVERY write. Bookkeeping columns on events would
-- bump updated_at, rewrite the search vector and take the hot event row's lock
-- each time the worker recorded progress — and any "has this been quiet for a
-- while?" check keyed off updated_at would be reset by that bookkeeping. A row
-- of its own touches none of that.
--
-- WHY SNAPSHOTS, NOT A HOOK IN EACH HANDLER
--
-- Same reasoning as 000082: several write paths, present and future. A worker
-- comparing "what ticket holders were last told" with "what the event says
-- now" covers all of them and is idempotent under retry.
-- ============================================================================

-- ── Watch state, one row per event ──────────────────────────────────────────

CREATE TABLE IF NOT EXISTS event_detail_watch (
    event_id           UUID        PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,

    -- What ticket holders were last told — or, for the baseline, what the
    -- event said when watching began. Always built by the SQL expression in
    -- the backfill below (mirrored in repository.detailsSnapshotSQL), never by
    -- application code, so jsonb equality against a fresh build is meaningful.
    announced_snapshot JSONB       NOT NULL,
    announced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- When a notice last actually went out. NULL until the first. Drives the
    -- minimum gap between notices for one event.
    notified_at        TIMESTAMPTZ,

    -- A change seen but not yet announced, and since when it has been stable.
    -- Announced only once the details stop moving for the settle window, so an
    -- organiser editing field by field produces one notice, not one per save.
    pending_snapshot   JSONB,
    pending_since      TIMESTAMPTZ,

    CONSTRAINT chk_edw_pending_pair
        CHECK ((pending_snapshot IS NULL) = (pending_since IS NULL))
);

-- ── One row per announcement ────────────────────────────────────────────────
--
-- What changed, frozen when it was announced. The record of what ticket
-- holders were told, and what every queued email renders from — so an email
-- sent ten minutes later describes the same change the push did, even if the
-- event has moved again since. (The newer change gets its own notice.)

CREATE TABLE IF NOT EXISTS event_update_notices (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id     UUID        NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    event_title  TEXT        NOT NULL,
    when_text    TEXT        NOT NULL,
    venue_text   TEXT        NOT NULL,
    changes      JSONB       NOT NULL,   -- models.EventDetailChanges
    summary      TEXT        NOT NULL,   -- the in-app / push body as sent
    in_app_count INTEGER     NOT NULL DEFAULT 0,
    email_count  INTEGER     NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_event_update_notices_event
    ON event_update_notices(event_id, created_at DESC);

-- ── Email outbox ────────────────────────────────────────────────────────────
--
-- SES is called one message at a time with a 30-second timeout, so emailing a
-- few thousand ticket holders inline would outlive any worker cycle and lose
-- its place on a restart. The announcement QUEUES the emails inside the same
-- transaction that records it; a drain loop sends them at a steady rate with
-- retries. Same idea as booking_email_retries.

CREATE TABLE IF NOT EXISTS event_update_emails (
    id              UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    notice_id       UUID         NOT NULL REFERENCES event_update_notices(id) ON DELETE CASCADE,
    event_id        UUID         NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    email           VARCHAR(255) NOT NULL,
    recipient_name  VARCHAR(255),

    -- Which link the email carries:
    --   account   /booking/:id                        signs in
    --   guest     /booking/:id?bn=&email=             no account to sign in to
    --   claimant  /claim-ticket?assignment=&email=    holds a ticket someone
    --                                                 else bought and assigned
    link_kind       VARCHAR(12)  NOT NULL,
    booking_id      UUID         NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    assignment_id   UUID         REFERENCES ticket_assignments(id) ON DELETE SET NULL,

    status          VARCHAR(12)  NOT NULL DEFAULT 'pending',
    attempts        INTEGER      NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_error      TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    sent_at         TIMESTAMPTZ,

    CONSTRAINT chk_eue_link_kind CHECK (link_kind IN ('account', 'guest', 'claimant')),
    -- 'sending' is a lease, not a terminal state: a row claimed by a process
    -- that died is picked up again once next_attempt_at passes.
    -- 'expired' is an email whose event ended before it could be delivered.
    CONSTRAINT chk_eue_status    CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'expired'))
);

-- One email per address per notice. Someone with two bookings, or who bought
-- one and was assigned a ticket from another, hears about a change once.
CREATE UNIQUE INDEX IF NOT EXISTS uq_eue_notice_email
    ON event_update_emails(notice_id, lower(email));

-- The drain loop's predicate.
CREATE INDEX IF NOT EXISTS idx_eue_due
    ON event_update_emails(next_attempt_at)
    WHERE status IN ('pending', 'sending');

-- ── Backfill ────────────────────────────────────────────────────────────────
--
-- Every event people currently hold tickets for starts with its CURRENT
-- details as already announced. Without this the worker's first run would read
-- every live event as changed and email every ticket holder on the platform
-- about nothing. 000078 and 000082 needed the same guard.
--
-- Drafts WITH confirmed bookings are included: that is an event taken down for
-- a fix. Its buyers keep their tickets, and whatever changes while it is down
-- should be announced when it goes live again — which needs a baseline from
-- before the change.
--
-- ⚠️ This expression MUST stay identical to detailsSnapshotSQL in
-- internal/repository/event_details_change_repository.go.
INSERT INTO event_detail_watch (event_id, announced_snapshot)
SELECT e.id,
       jsonb_build_object(
           'title',   e.title,
           'desc',    md5(lower(btrim(regexp_replace(e.description, '\s+', ' ', 'g')))),
           'start',   floor(extract(epoch FROM e.start_time))::bigint,
           'end',     floor(extract(epoch FROM e.end_time))::bigint,
           'tz',      COALESCE(e.timezone, ''),
           'venue',   COALESCE(e.venue_name, ''),
           'address', COALESCE(e.address, ''),
           'city',    COALESCE(e.city, ''),
           'info',    COALESCE(
                        (SELECT jsonb_object_agg(i.info_key, i.value)
                         FROM   event_info_items i
                         WHERE  i.event_id = e.id),
                        '{}'::jsonb)
       )
FROM   events e
WHERE  e.status IN ('published', 'published_private')
   OR (e.status = 'draft' AND EXISTS (
           SELECT 1
           FROM   bookings b
           WHERE  b.event_id = e.id
             AND  b.booking_status = 'confirmed'))
ON CONFLICT (event_id) DO NOTHING;