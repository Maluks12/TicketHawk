package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Event detail change notifications — watch state, audience, email outbox.
//
// Schema and the reasons for a side table: migrations/000092.
// Flow and timing: workers/event_details_change_worker.go.
// What counts as a change and how it is worded: models/event_details_change.go.
// ─────────────────────────────────────────────────────────────────────────────

// detailsSnapshotSQL builds the details snapshot for `events e`.
//
// ⚠️ MUST stay identical to the backfill in migrations/000092. The worker
// compares a stored snapshot to a fresh one with jsonb equality; if the two
// expressions drift, every live event reads as changed.
//
// '\s+' is a Postgres regex, written literally — this is a Go raw string.
const detailsSnapshotSQL = `jsonb_build_object(
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
)`

// ── Candidate detection ──────────────────────────────────────────────────────

// DetailWatchTiming carries the worker's timing rules into the candidate query,
// so every time comparison uses the DATABASE clock — the same clock that
// stamped pending_since and notified_at.
type DetailWatchTiming struct {
	Settle     time.Duration // details must be unchanged this long before announcing
	MinGap     time.Duration // at most one notice per event per MinGap...
	MinGapSoon time.Duration // ...or per MinGapSoon once the event is within SoonWindow
	SoonWindow time.Duration
}

// DetailWatchCandidate is a live, upcoming event whose details differ from
// what was last announced, has a pending change, or has never been watched.
type DetailWatchCandidate struct {
	EventID         string
	EventSlug       string
	OrganizerUserID string

	// Raw snapshots exactly as the database built them. Written back
	// byte-for-byte, never re-marshalled, so the jsonb equality the next cycle
	// relies on holds.
	AnnouncedRaw []byte
	CurrentRaw   []byte
	Announced    models.EventDetailsSnapshot
	Current      models.EventDetailsSnapshot

	// NeverAnnounced: no watch row — published after 000092, or never seen.
	NeverAnnounced bool
	// BadAnnounced / BadCurrent: a snapshot that did not parse. Should never
	// happen (both are built by the same SQL); the worker re-baselines rather
	// than diffing garbage into a notification.
	BadAnnounced bool
	BadCurrent   bool

	// PendingMatches: a pending change is recorded and the event still says
	// exactly that — unchanged since the settle clock started.
	PendingMatches bool
	// Settled: the pending change has been stable for the settle window.
	Settled bool
	// GapClear: the minimum gap since this event's last notice has passed.
	GapClear bool
}

// FindChangedEventDetails returns events that need the worker's attention,
// soonest first — a change to tonight's event matters more than one to an
// event in March.
//
// The cheap part is done here: which events are live and upcoming, their
// current snapshot, and an exact jsonb comparison that skips every event
// nothing has happened to. Whether a difference is worth telling anyone about
// (case, whitespace, sub-minute times) is decided by models.DiffEventDetails.
func FindChangedEventDetails(
	pool *pgxpool.Pool,
	ctx context.Context,
	timing DetailWatchTiming,
	limit int,
) ([]DetailWatchCandidate, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := pool.Query(ctx, `
		WITH cur AS (
			SELECT e.id, e.slug, e.organizer_id, e.start_time,
			       `+detailsSnapshotSQL+` AS snap
			FROM   events e
			WHERE  e.status IN ('published', 'published_private')
			  -- A change to a finished event is a records correction.
			  AND  e.end_time >= NOW()
		)
		SELECT c.id::text,
		       COALESCE(c.slug, ''),
		       c.organizer_id::text,
		       (w.event_id IS NULL),
		       COALESCE(w.announced_snapshot, '{}'::jsonb)::text,
		       c.snap::text,
		       COALESCE(w.pending_snapshot = c.snap, FALSE),
		       COALESCE(w.pending_since <= NOW() - $1::interval, FALSE),
		       (w.notified_at IS NULL
		        OR w.notified_at <= NOW() - CASE
		             WHEN c.start_time <= NOW() + $3::interval THEN $4::interval
		             ELSE $2::interval
		           END)
		FROM   cur c
		LEFT   JOIN event_detail_watch w ON w.event_id = c.id
		WHERE  w.event_id IS NULL
		   OR  w.announced_snapshot <> c.snap
		   -- A pending change the organiser has since reverted must still be
		   -- visited, so the worker can clear it.
		   OR  w.pending_snapshot IS NOT NULL
		ORDER  BY c.start_time
		LIMIT  $5
	`,
		edcInterval(timing.Settle),
		edcInterval(timing.MinGap),
		edcInterval(timing.SoonWindow),
		edcInterval(timing.MinGapSoon),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("FindChangedEventDetails: %w", err)
	}
	defer rows.Close()

	out := make([]DetailWatchCandidate, 0, limit)
	for rows.Next() {
		var (
			c                  DetailWatchCandidate
			announced, current string
		)
		if err := rows.Scan(
			&c.EventID, &c.EventSlug, &c.OrganizerUserID,
			&c.NeverAnnounced, &announced, &current,
			&c.PendingMatches, &c.Settled, &c.GapClear,
		); err != nil {
			return nil, fmt.Errorf("FindChangedEventDetails scan: %w", err)
		}
		c.AnnouncedRaw = []byte(announced)
		c.CurrentRaw = []byte(current)
		if err := json.Unmarshal(c.AnnouncedRaw, &c.Announced); err != nil {
			c.BadAnnounced = true
		}
		if err := json.Unmarshal(c.CurrentRaw, &c.Current); err != nil {
			c.BadCurrent = true
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── Watch state ──────────────────────────────────────────────────────────────

// SetEventDetailBaseline records a snapshot as already announced WITHOUT
// notifying anyone.
//
// Used when an event is first seen live: its details were assembled while it
// was a draft, nobody has been told anything, so there is nothing to have
// changed. Also used to recover a snapshot that failed to parse.
func SetEventDetailBaseline(pool *pgxpool.Pool, ctx context.Context, eventID string, snapshot []byte) error {
	if _, err := pool.Exec(ctx, `
		INSERT INTO event_detail_watch (event_id, announced_snapshot)
		VALUES ($1, $2::jsonb)
		ON CONFLICT (event_id) DO UPDATE
		SET    announced_snapshot = EXCLUDED.announced_snapshot,
		       announced_at       = NOW(),
		       pending_snapshot   = NULL,
		       pending_since      = NULL
	`, eventID, snapshot); err != nil {
		return fmt.Errorf("SetEventDetailBaseline: %w", err)
	}
	return nil
}

// AcceptEventDetailsSilently moves the baseline to current without a notice.
//
// For differences that are not worth telling anyone about — case and
// whitespace edits — and for a pending change the organiser reverted.
// notified_at is untouched: nothing was sent, so the gap is not spent.
//
// Compare-and-swap on the baseline the decision was made against, so it can
// never overwrite a notice another process has just recorded.
func AcceptEventDetailsSilently(pool *pgxpool.Pool, ctx context.Context, eventID string, expected, current []byte) error {
	if _, err := pool.Exec(ctx, `
		UPDATE event_detail_watch
		SET    announced_snapshot = $3::jsonb,
		       announced_at       = NOW(),
		       pending_snapshot   = NULL,
		       pending_since      = NULL
		WHERE  event_id = $1
		  AND  announced_snapshot = $2::jsonb
	`, eventID, expected, current); err != nil {
		return fmt.Errorf("AcceptEventDetailsSilently: %w", err)
	}
	return nil
}

// MarkEventDetailsPending records a change and (re)starts its settle clock.
//
// Called whenever the details differ from what is pending — so every further
// edit restarts the clock, and a notice goes out only once the organiser has
// stopped for the whole settle window.
func MarkEventDetailsPending(pool *pgxpool.Pool, ctx context.Context, eventID string, current []byte) error {
	if _, err := pool.Exec(ctx, `
		UPDATE event_detail_watch
		SET    pending_snapshot = $2::jsonb,
		       pending_since    = NOW()
		WHERE  event_id = $1
	`, eventID, current); err != nil {
		return fmt.Errorf("MarkEventDetailsPending: %w", err)
	}
	return nil
}

// EventHasTicketHolders reports whether anyone holds a confirmed TICKET booking.
//
// Merch-only bookings do not count: buying a T-shirt is not a stake in the
// event's start time.
func EventHasTicketHolders(pool *pgxpool.Pool, ctx context.Context, eventID string) (bool, error) {
	var ok bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM   bookings
			WHERE  event_id = $1
			  AND  booking_status = 'confirmed'
			  AND  booking_type = 'ticket'
		)
	`, eventID).Scan(&ok); err != nil {
		return false, fmt.Errorf("EventHasTicketHolders: %w", err)
	}
	return ok, nil
}

// ── Announcing ───────────────────────────────────────────────────────────────

// EventDetailsAnnouncement is everything needed to record and fan out one
// notice.
type EventDetailsAnnouncement struct {
	EventID         string
	OrganizerUserID string

	Expected []byte // the announced snapshot the diff was computed against
	Current  []byte // the snapshot being announced

	EventTitle string
	WhenText   string
	VenueText  string
	Changes    models.EventDetailChanges

	Title   string // notification title
	Body    string // notification body
	Payload models.EventDetailsChangePayload
}

// EventDetailsAnnounced is what an announcement did.
type EventDetailsAnnounced struct {
	NoticeID     string
	InAppUserIDs []string
	EmailsQueued int
	// PushData is the notification Data exactly as written to the rows, for
	// DispatchNotificationPushes.
	PushData json.RawMessage
	// Superseded: the baseline was no longer the one the diff was computed
	// against — another process announced first. Nothing was written.
	Superseded bool
}

// AnnounceEventDetailsChange records a notice and fans it out, in ONE
// transaction:
//
//  1. moves the baseline to the announced snapshot (compare-and-swap)
//  2. writes the notice row
//  3. writes an in-app notification for every account holding a ticket
//  4. queues one email per address holding a ticket — accounts, guests and
//     people who claimed an assigned ticket
//
// All or nothing. The line-up worker sends one INSERT per recipient and
// advances its snapshot afterwards, so a failure half way re-notifies everyone
// already reached on the next cycle. Here a failure rolls back the whole
// notice, the baseline stays put, and the next cycle retries it cleanly.
//
// Pushes are NOT sent here. They need the rows committed first — a buzz for a
// row that then rolls back would open an empty inbox — so the caller passes
// the returned ids to DispatchNotificationPushes.
func AnnounceEventDetailsChange(
	pool *pgxpool.Pool,
	ctx context.Context,
	a EventDetailsAnnouncement,
) (EventDetailsAnnounced, error) {
	var res EventDetailsAnnounced

	changesJSON, err := json.Marshal(a.Changes)
	if err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: encode changes: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	// ── 1. Claim ─────────────────────────────────────────────────────────────
	tag, err := tx.Exec(ctx, `
		UPDATE event_detail_watch
		SET    announced_snapshot = $3::jsonb,
		       announced_at       = NOW(),
		       notified_at        = NOW(),
		       pending_snapshot   = NULL,
		       pending_since      = NULL
		WHERE  event_id = $1
		  AND  announced_snapshot = $2::jsonb
	`, a.EventID, a.Expected, a.Current)
	if err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: claim: %w", err)
	}
	if tag.RowsAffected() == 0 {
		res.Superseded = true
		return res, nil
	}

	// ── 2. Notice ────────────────────────────────────────────────────────────
	if err := tx.QueryRow(ctx, `
		INSERT INTO event_update_notices
		       (event_id, event_title, when_text, venue_text, changes, summary)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		RETURNING id::text
	`, a.EventID, a.EventTitle, a.WhenText, a.VenueText, changesJSON, a.Body).Scan(&res.NoticeID); err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: notice: %w", err)
	}

	a.Payload.NoticeID = res.NoticeID
	payloadJSON, err := json.Marshal(a.Payload)
	if err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: encode payload: %w", err)
	}
	res.PushData = payloadJSON

	organiser := edcNullableID(a.OrganizerUserID)

	// ── 3. In-app ────────────────────────────────────────────────────────────
	//
	// Accounts holding a confirmed ticket booking. Deleted accounts are
	// skipped: account deletion removes the inbox as personal content
	// (000065), so a new row for one is a row nobody can ever read. The
	// organiser is skipped — they made the change.
	rows, err := tx.Query(ctx, `
		INSERT INTO notifications (user_id, type, title, body, data)
		SELECT h.user_id, $2::text, $3::text, $4::text, $5::jsonb
		FROM (
			SELECT DISTINCT b.user_id
			FROM   bookings b
			JOIN   users u ON u.id = b.user_id
			WHERE  b.event_id = $1
			  AND  b.booking_status = 'confirmed'
			  AND  b.booking_type = 'ticket'
			  AND  u.deleted_at IS NULL
			  AND  ($6::uuid IS NULL OR b.user_id <> $6::uuid)
		) h
		RETURNING user_id::text
	`, a.EventID, string(models.NotifEventDetailsChanged), a.Title, a.Body, payloadJSON, organiser)
	if err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: in-app: %w", err)
	}
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return res, fmt.Errorf("AnnounceEventDetailsChange: in-app scan: %w", err)
		}
		res.InAppUserIDs = append(res.InAppUserIDs, uid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: in-app rows: %w", err)
	}

	// ── 4. Emails ────────────────────────────────────────────────────────────
	//
	// Three ways to hold a ticket, one email per address:
	//   account   the account's own address — where the booking confirmation
	//             went
	//   guest     the address given at checkout; no account, so email is the
	//             ONLY way to reach them
	//   claimant  someone who accepted a ticket assigned from another booking;
	//             they are the person walking up to the gate
	//
	// DISTINCT ON keeps the best link per address: an account's booking page
	// over a guest link over a claim link, then the earliest booking.
	tag, err = tx.Exec(ctx, `
		INSERT INTO event_update_emails
		       (notice_id, event_id, email, recipient_name, link_kind, booking_id, assignment_id)
		SELECT DISTINCT ON (lower(r.email))
		       $1::uuid, $2::uuid, r.email, r.name, r.link_kind, r.booking_id, r.assignment_id
		FROM (
			SELECT btrim(u.email)                                              AS email,
			       NULLIF(btrim(concat_ws(' ', u.first_name, u.last_name)), '') AS name,
			       'account'                                                   AS link_kind,
			       b.id                                                        AS booking_id,
			       NULL::uuid                                                  AS assignment_id,
			       1                                                           AS priority,
			       b.created_at                                                AS since
			FROM   bookings b
			JOIN   users u ON u.id = b.user_id
			WHERE  b.event_id = $2
			  AND  b.booking_status = 'confirmed'
			  AND  b.booking_type = 'ticket'
			  AND  u.deleted_at IS NULL
			  AND  ($3::uuid IS NULL OR b.user_id <> $3::uuid)

			UNION ALL

			SELECT btrim(b.attendee_email),
			       NULLIF(btrim(concat_ws(' ', b.attendee_first_name, b.attendee_last_name)), ''),
			       'guest', b.id, NULL::uuid, 2, b.created_at
			FROM   bookings b
			WHERE  b.event_id = $2
			  AND  b.booking_status = 'confirmed'
			  AND  b.booking_type = 'ticket'
			  AND  b.user_id IS NULL
			  AND  COALESCE(btrim(b.attendee_email), '') <> ''

			UNION ALL

			SELECT btrim(ta.assignee_email),
			       NULLIF(btrim(ta.assignee_name), ''),
			       'claimant', b.id, ta.id, 3, COALESCE(ta.claimed_at, ta.created_at)
			FROM   ticket_assignments ta
			JOIN   bookings b ON b.id = ta.booking_id
			WHERE  b.event_id = $2
			  AND  b.booking_status = 'confirmed'
			  AND  b.booking_type = 'ticket'
			  AND  ta.status = 'claimed'
			  AND  COALESCE(btrim(ta.assignee_email), '') <> ''
		) r
		WHERE  r.email LIKE '%_@_%'
		  AND  lower(r.email) NOT LIKE '%@deleted.invalid'
		ORDER  BY lower(r.email), r.priority, r.since
		ON CONFLICT (notice_id, (lower(email))) DO NOTHING
	`, res.NoticeID, a.EventID, organiser)
	if err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: emails: %w", err)
	}
	res.EmailsQueued = int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `
		UPDATE event_update_notices
		SET    in_app_count = $2, email_count = $3
		WHERE  id = $1
	`, res.NoticeID, len(res.InAppUserIDs), res.EmailsQueued); err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: counts: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("AnnounceEventDetailsChange: commit: %w", err)
	}
	return res, nil
}

// DispatchNotificationPushes hands already-committed notification rows to the
// push service, paced.
//
// The push queue holds 4096 jobs and DROPS overflow (PushService.Dispatch) —
// the right trade for per-row producers, which pace themselves one INSERT at a
// time. A set-based insert hands back every recipient at once; pushing twenty
// thousand ids into that queue in one loop would drop most of them. Pausing
// between chunks lets the sender drain.
//
// Stops early if ctx ends. The in-app rows are already committed, so an
// interrupted fan-out costs buzzes, never notifications.
func DispatchNotificationPushes(
	ctx context.Context,
	userIDs []string,
	notifType models.NotificationType,
	title, body string,
	data json.RawMessage,
	chunk int,
	pause time.Duration,
) {
	p := pushDispatcher.Load()
	if p == nil || len(userIDs) == 0 {
		return
	}
	if chunk <= 0 {
		chunk = 400
	}
	for i, uid := range userIDs {
		if i > 0 && i%chunk == 0 {
			t := time.NewTimer(pause)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
		(*p)(uid, notifType, title, body, data)
	}
}

// ── Email outbox ─────────────────────────────────────────────────────────────

// EventUpdateEmailJob is one claimed email, with everything needed to render
// it: the notice it belongs to is joined in, frozen as it was announced.
type EventUpdateEmailJob struct {
	ID            string
	NoticeID      string
	EventID       string
	Email         string
	Name          string
	LinkKind      string
	BookingID     string
	BookingNumber string
	AssignmentID  string
	// Attempts INCLUDING this one.
	Attempts int

	EventTitle string
	WhenText   string
	VenueText  string
	Changes    models.EventDetailChanges

	// EventEnded: the event finished before this email went out. "The time
	// has changed" arriving after the event is noise; the worker expires it.
	EventEnded bool
}

// ClaimDueEventUpdateEmails takes up to limit due emails and leases them.
//
// The lease is status 'sending' plus next_attempt_at pushed forward. A process
// that dies mid-batch leaves its rows leased; once next_attempt_at passes they
// are due again and the next drain picks them up. SKIP LOCKED means two drains
// never take the same row.
func ClaimDueEventUpdateEmails(
	pool *pgxpool.Pool,
	ctx context.Context,
	limit int,
	lease time.Duration,
) ([]EventUpdateEmailJob, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := pool.Query(ctx, `
		WITH due AS (
			SELECT q.id
			FROM   event_update_emails q
			WHERE  q.status IN ('pending', 'sending')
			  AND  q.next_attempt_at <= NOW()
			ORDER  BY q.created_at, q.id
			LIMIT  $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE event_update_emails q
		SET    status          = 'sending',
		       attempts        = q.attempts + 1,
		       next_attempt_at = NOW() + $2::interval
		FROM   due, event_update_notices n, events e, bookings b
		WHERE  q.id = due.id
		  AND  n.id = q.notice_id
		  AND  e.id = q.event_id
		  AND  b.id = q.booking_id
		RETURNING q.id::text, q.notice_id::text, q.event_id::text,
		          q.email, COALESCE(q.recipient_name, ''), q.link_kind,
		          q.booking_id::text, COALESCE(b.booking_number, ''),
		          COALESCE(q.assignment_id::text, ''),
		          q.attempts,
		          n.event_title, n.when_text, n.venue_text, n.changes::text,
		          (e.end_time < NOW())
	`, limit, edcInterval(lease))
	if err != nil {
		return nil, fmt.Errorf("ClaimDueEventUpdateEmails: %w", err)
	}
	defer rows.Close()

	out := make([]EventUpdateEmailJob, 0, limit)
	for rows.Next() {
		var (
			j       EventUpdateEmailJob
			changes string
		)
		if err := rows.Scan(
			&j.ID, &j.NoticeID, &j.EventID,
			&j.Email, &j.Name, &j.LinkKind,
			&j.BookingID, &j.BookingNumber,
			&j.AssignmentID,
			&j.Attempts,
			&j.EventTitle, &j.WhenText, &j.VenueText, &changes,
			&j.EventEnded,
		); err != nil {
			return nil, fmt.Errorf("ClaimDueEventUpdateEmails scan: %w", err)
		}
		// A notice whose change list will not parse still gets its email —
		// the header, current details and link are all intact.
		_ = json.Unmarshal([]byte(changes), &j.Changes)
		out = append(out, j)
	}
	return out, rows.Err()
}

// MarkEventUpdateEmailSent records a delivered email.
func MarkEventUpdateEmailSent(pool *pgxpool.Pool, ctx context.Context, id string) error {
	if _, err := pool.Exec(ctx, `
		UPDATE event_update_emails
		SET    status = 'sent', sent_at = NOW(), last_error = NULL
		WHERE  id = $1
	`, id); err != nil {
		return fmt.Errorf("MarkEventUpdateEmailSent: %w", err)
	}
	return nil
}

// RetryEventUpdateEmail puts a failed email back in the queue after delay.
func RetryEventUpdateEmail(pool *pgxpool.Pool, ctx context.Context, id string, delay time.Duration, reason string) error {
	if _, err := pool.Exec(ctx, `
		UPDATE event_update_emails
		SET    status = 'pending', next_attempt_at = NOW() + $2::interval, last_error = $3
		WHERE  id = $1
	`, id, edcInterval(delay), edcTruncate(reason, 1000)); err != nil {
		return fmt.Errorf("RetryEventUpdateEmail: %w", err)
	}
	return nil
}

// CloseEventUpdateEmail ends an email without sending it: 'failed' (permanent
// rejection or out of attempts — left for review, like booking_email_retries)
// or 'expired' (the event ended first).
func CloseEventUpdateEmail(pool *pgxpool.Pool, ctx context.Context, id, status, reason string) error {
	if status != "failed" && status != "expired" {
		return fmt.Errorf("CloseEventUpdateEmail: %q is not a closing status", status)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE event_update_emails
		SET    status = $2, last_error = $3
		WHERE  id = $1
	`, id, status, edcTruncate(reason, 1000)); err != nil {
		return fmt.Errorf("CloseEventUpdateEmail: %w", err)
	}
	return nil
}

// PruneEventUpdateEmails deletes delivered and expired rows older than keep.
//
// One notice to a sold-out venue is thousands of rows. The notice row itself
// is kept — that is the record of what people were told — and failed rows are
// kept for review.
func PruneEventUpdateEmails(pool *pgxpool.Pool, ctx context.Context, keep time.Duration) (int64, error) {
	tag, err := pool.Exec(ctx, `
		DELETE FROM event_update_emails
		WHERE  status IN ('sent', 'expired')
		  AND  created_at < NOW() - $1::interval
	`, edcInterval(keep))
	if err != nil {
		return 0, fmt.Errorf("PruneEventUpdateEmails: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────
//
// Prefixed so they cannot collide with the many small helpers already in this
// package (nullIfEmpty, strptr, ...).

// edcInterval renders a duration as a Postgres interval literal.
func edcInterval(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d/time.Second))
}

// edcNullableID is nil for an empty id, so a `$n::uuid IS NULL` guard works.
func edcNullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// edcTruncate trims s to at most n runes for an error column.
func edcTruncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}