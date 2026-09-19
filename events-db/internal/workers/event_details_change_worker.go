package workers

import (
	"context"
	"strings"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const (
	// detailsPollInterval — how often to look for changed details.
	detailsPollInterval = 2 * time.Minute

	// detailsSettleWindow — how long the details must sit unchanged before a
	// change is announced. An organiser fixing the parking note, then the doors
	// time, then the dress code saves three times; this turns that into one
	// notice. Measured from the LAST edit, on the details themselves — not on
	// events.updated_at, which the updated_at trigger moves on every write.
	detailsSettleWindow = 5 * time.Minute

	// detailsMinGap — at most one notice per event per hour. Changes made
	// inside the gap are not lost: they accumulate against the last announced
	// baseline and go out together when it opens.
	detailsMinGap = 1 * time.Hour

	// detailsMinGapSoon / detailsSoonWindow — within a day of the start, a late
	// change ("north gate closed, use the south entrance") can't sit for an
	// hour, so the gap shrinks.
	detailsMinGapSoon = 15 * time.Minute
	detailsSoonWindow = 24 * time.Hour

	detailsEventsPerCycle = 50

	// Push pacing. The push queue holds 4096 and drops overflow; see
	// repository.DispatchNotificationPushes.
	detailsPushChunk = 400
	detailsPushPause = 500 * time.Millisecond

	// Email drain. ~8 emails a second at most, so a sold-out venue's notice
	// does not crowd out booking confirmations on the same SES account.
	updateEmailDrainInterval = 20 * time.Second
	updateEmailBatch         = 50
	updateEmailLease         = 10 * time.Minute
	updateEmailSpacing       = 120 * time.Millisecond
	updateEmailDrainBudget   = 2 * time.Minute
	updateEmailMaxAttempts   = 6
	updateEmailRetention     = 90 * 24 * time.Hour
)

// updateEmailBackoffs is indexed by attempts already made (1 → first entry).
var updateEmailBackoffs = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
}

// StartEventDetailsChangeWorker tells ticket holders when a live event's
// details change: date and time, venue, name, description, or any field in
// its Information block.
//
// ── Who hears, and how ──────────────────────────────────────────────────────
//
//	accounts holding a ticket   in-app notification + push, and email
//	guests                      email (no account, so no inbox — the gap the
//	                            line-up worker left open)
//	people with a claimed       email, with their own ticket link
//	assigned ticket
//
// The organiser is never notified about their own edit.
//
// ── Why a worker, not a hook in the save handlers ───────────────────────────
//
// Same reasoning as the line-up worker: the details are written by the
// organiser's info PUT, by the admin edit-request executor, and by a draft
// edit while an event is taken down. Comparing state — "what were people last
// told?" against "what does the event say now?" — covers all of them, keeps
// the fan-out off the organiser's save request, and is idempotent under retry.
//
// ── Two loops ───────────────────────────────────────────────────────────────
//
// Detection announces a change in one transaction: notice row, in-app rows
// and queued emails together. Then a separate drain sends the queued emails at
// a steady rate with retries, so SES speed never holds up detection and a
// restart never loses its place.
func StartEventDetailsChangeWorker(pool *pgxpool.Pool, cfg *config.Config, ctx context.Context) {
	emailSvc := services.NewEmailService(cfg)

	go func() {
		log.Info().
			Dur("interval", detailsPollInterval).
			Dur("settle", detailsSettleWindow).
			Msg("event details worker: detection started")

		ticker := time.NewTicker(detailsPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runDetailsChangeCycle(pool, ctx)
			case <-ctx.Done():
				log.Info().Msg("event details worker: detection stopped")
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(updateEmailDrainInterval)
		defer ticker.Stop()
		var lastPrune time.Time

		for {
			select {
			case <-ticker.C:
				drainEventUpdateEmails(ctx, pool, emailSvc)

				if time.Since(lastPrune) >= time.Hour {
					lastPrune = time.Now()
					pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
					if n, err := repository.PruneEventUpdateEmails(pool, pctx, updateEmailRetention); err != nil {
						log.Warn().Err(err).Msg("event details worker: email prune failed")
					} else if n > 0 {
						log.Info().Int64("deleted", n).Msg("event details worker: pruned old update emails")
					}
					cancel()
				}
			case <-ctx.Done():
				log.Info().Msg("event details worker: email drain stopped")
				return
			}
		}
	}()
}

// ── Detection ────────────────────────────────────────────────────────────────

func runDetailsChangeCycle(pool *pgxpool.Pool, ctx context.Context) {
	cycleCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	candidates, err := repository.FindChangedEventDetails(pool, cycleCtx, repository.DetailWatchTiming{
		Settle:     detailsSettleWindow,
		MinGap:     detailsMinGap,
		MinGapSoon: detailsMinGapSoon,
		SoonWindow: detailsSoonWindow,
	}, detailsEventsPerCycle)
	if err != nil {
		log.Error().Err(err).Msg("event details worker: candidate query failed")
		return
	}
	if len(candidates) == 0 {
		return
	}

	// Labels for the Information fields. Loaded once per cycle, only when
	// something needs diffing. A failure is survivable: DiffEventDetails falls
	// back to readable versions of the keys.
	var catalogue map[string]models.EventInfoCatalogueEntry
	catalogueLoaded := false

	for _, c := range candidates {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// ── First sight: baseline, silently ─────────────────────────────────
		//
		// Published after 000092 shipped. Its details were assembled as a
		// draft and nobody has been told anything, so there is nothing to
		// have changed. The organiser follow announcement covers publication.
		if c.NeverAnnounced || c.BadAnnounced {
			if c.BadCurrent {
				log.Error().Str("event_id", c.EventID).
					Msg("event details worker: current snapshot unreadable — skipping")
				continue
			}
			if err := repository.SetEventDetailBaseline(pool, cycleCtx, c.EventID, c.CurrentRaw); err != nil {
				log.Warn().Err(err).Str("event_id", c.EventID).
					Msg("event details worker: could not record baseline")
			}
			continue
		}
		if c.BadCurrent {
			log.Error().Str("event_id", c.EventID).
				Msg("event details worker: current snapshot unreadable — skipping")
			continue
		}

		if !catalogueLoaded {
			catalogueLoaded = true
			if catalogue, err = repository.GetInfoCatalogueMap(pool); err != nil {
				log.Warn().Err(err).Msg("event details worker: catalogue unavailable — using key names as labels")
				catalogue = nil
			}
		}

		changes := models.DiffEventDetails(c.Announced, c.Current, catalogue, formatEventWhen)

		// ── Nothing worth saying ────────────────────────────────────────────
		//
		// The snapshots differ byte-wise, but only by case, whitespace or
		// seconds — or a pending change was reverted before it settled. Move
		// the baseline so this event stops being revisited.
		if len(changes) == 0 {
			if err := repository.AcceptEventDetailsSilently(pool, cycleCtx, c.EventID, c.AnnouncedRaw, c.CurrentRaw); err != nil {
				log.Warn().Err(err).Str("event_id", c.EventID).
					Msg("event details worker: could not refresh baseline")
			}
			continue
		}

		// ── Still moving: (re)start the settle clock ───────────────────────
		if !c.PendingMatches {
			if err := repository.MarkEventDetailsPending(pool, cycleCtx, c.EventID, c.CurrentRaw); err != nil {
				log.Warn().Err(err).Str("event_id", c.EventID).
					Msg("event details worker: could not record pending change")
			}
			continue
		}

		// ── Stable, but not for long enough, or too soon after the last ────
		if !c.Settled || !c.GapClear {
			continue
		}

		// ── Nobody to tell ──────────────────────────────────────────────────
		//
		// Most live events are edited before anyone has bought. Accept the
		// change without spending the gap on a notice that reaches no one.
		// Anyone who books afterwards books against the new details.
		has, err := repository.EventHasTicketHolders(pool, cycleCtx, c.EventID)
		if err != nil {
			log.Warn().Err(err).Str("event_id", c.EventID).
				Msg("event details worker: ticket holder check failed")
			continue
		}
		if !has {
			if err := repository.AcceptEventDetailsSilently(pool, cycleCtx, c.EventID, c.AnnouncedRaw, c.CurrentRaw); err != nil {
				log.Warn().Err(err).Str("event_id", c.EventID).
					Msg("event details worker: could not refresh baseline")
			}
			continue
		}

		announceDetailsChange(pool, ctx, cycleCtx, c, changes)
	}
}

// announceDetailsChange records the notice, then sends the pushes.
//
// ctx (the worker's) paces the pushes rather than cycleCtx, so a large
// fan-out is not cut short by the cycle timeout.
func announceDetailsChange(
	pool *pgxpool.Pool,
	ctx, cycleCtx context.Context,
	c repository.DetailWatchCandidate,
	changes models.EventDetailChanges,
) {
	// Everything in the notice is taken from the snapshot being announced, not
	// re-read from the event, so the header, the change list and the stored
	// baseline all describe the same moment.
	cur := c.Current
	title := strings.TrimSpace(cur.Title)
	whenFull, _ := formatEventWhen(time.Unix(cur.Start, 0), time.Unix(cur.End, 0), cur.Timezone)
	venue := models.VenueText(cur.Venue, cur.Address, cur.City)

	notifTitle := changes.NotificationTitle()
	notifBody := changes.NotificationBody(title)

	res, err := repository.AnnounceEventDetailsChange(pool, cycleCtx, repository.EventDetailsAnnouncement{
		EventID:         c.EventID,
		OrganizerUserID: c.OrganizerUserID,
		Expected:        c.AnnouncedRaw,
		Current:         c.CurrentRaw,
		EventTitle:      title,
		WhenText:        whenFull,
		VenueText:       venue,
		Changes:         changes,
		Title:           notifTitle,
		Body:            notifBody,
		Payload: models.EventDetailsChangePayload{
			EventID:      c.EventID,
			EventSlug:    c.EventSlug,
			EventTitle:   title,
			Fields:       changes.Fields(),
			TimeChanged:  changes.TimeChanged(),
			VenueChanged: changes.VenueChanged(),
		},
	})
	if err != nil {
		// Rolled back as a whole: no notice, no rows, baseline unmoved. The
		// next cycle recomputes the same diff and tries again.
		log.Error().Err(err).Str("event_id", c.EventID).
			Msg("event details worker: announcement failed — will retry")
		return
	}
	if res.Superseded {
		log.Info().Str("event_id", c.EventID).
			Msg("event details worker: already announced elsewhere — skipped")
		return
	}

	log.Info().
		Str("event_id", c.EventID).
		Str("event", title).
		Str("notice_id", res.NoticeID).
		Strs("fields", changes.Fields()).
		Int("in_app", len(res.InAppUserIDs)).
		Int("emails_queued", res.EmailsQueued).
		Msg("event details worker: change announced")

	repository.DispatchNotificationPushes(
		ctx, res.InAppUserIDs, models.NotifEventDetailsChanged,
		notifTitle, notifBody, res.PushData,
		detailsPushChunk, detailsPushPause,
	)
}

// formatEventWhen renders start and end in the event's own timezone.
//
//	full   "Saturday, 14 November 2026 at 18:00 – 23:00"
//	       "Saturday, 14 November 2026 at 22:00 – Sunday, 15 November at 04:00"
//	short  "Sat 14 Nov, 18:00"
//
// Built on the timefmt layouts so emails read the same as every other event
// time the platform sends, in SAST rather than the server's UTC.
func formatEventWhen(start, end time.Time, tz string) (full, short string) {
	loc := utils.EventLocation(tz)
	s, e := start.In(loc), end.In(loc)

	var endText string
	switch {
	case s.Year() == e.Year() && s.YearDay() == e.YearDay():
		endText = e.Format("15:04")
	case s.Year() == e.Year():
		endText = e.Format(utils.EventDayLayout + " at 15:04")
	default:
		endText = e.Format(utils.EventDateTimeLayout)
	}

	full = s.Format(utils.EventDateTimeLayout) + " – " + endText
	short = s.Format("Mon 2 Jan, 15:04")
	return full, short
}

// ── Email drain ──────────────────────────────────────────────────────────────

// drainEventUpdateEmails sends due emails until none are due or the budget is
// spent; the next tick carries on.
func drainEventUpdateEmails(ctx context.Context, pool *pgxpool.Pool, emailSvc *services.EmailService) {
	deadline := time.Now().Add(updateEmailDrainBudget)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}

		claimCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		jobs, err := repository.ClaimDueEventUpdateEmails(pool, claimCtx, updateEmailBatch, updateEmailLease)
		cancel()
		if err != nil {
			log.Error().Err(err).Msg("event details worker: could not claim update emails")
			return
		}
		if len(jobs) == 0 {
			return
		}

		for i, j := range jobs {
			if ctx.Err() != nil {
				// Unsent rows keep their lease and are picked up after it
				// expires — at most a short delay, never a loss.
				return
			}
			if i > 0 {
				t := time.NewTimer(updateEmailSpacing)
				select {
				case <-ctx.Done():
					t.Stop()
					return
				case <-t.C:
				}
			}
			sendEventUpdateEmail(pool, emailSvc, j)
		}
	}
}

// sendEventUpdateEmail sends one queued email and records the outcome.
func sendEventUpdateEmail(pool *pgxpool.Pool, emailSvc *services.EmailService, j repository.EventUpdateEmailJob) {
	// Bookkeeping gets its own short context, independent of shutdown: a sent
	// email that is not marked sent would be sent again after the lease.
	opCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if j.EventEnded {
		if err := repository.CloseEventUpdateEmail(pool, opCtx, j.ID, "expired", "event ended before delivery"); err != nil {
			log.Warn().Err(err).Str("email_id", j.ID).Msg("event details worker: could not expire email")
		}
		return
	}

	err := emailSvc.SendEventUpdate(services.EventUpdateEmail{
		ToEmail:       j.Email,
		ToName:        j.Name,
		EventID:       j.EventID,
		EventTitle:    j.EventTitle,
		WhenText:      j.WhenText,
		VenueText:     j.VenueText,
		Changes:       j.Changes,
		LinkKind:      j.LinkKind,
		BookingID:     j.BookingID,
		BookingNumber: j.BookingNumber,
		AssignmentID:  j.AssignmentID,
	})

	switch {
	case err == nil:
		if err := repository.MarkEventUpdateEmailSent(pool, opCtx, j.ID); err != nil {
			log.Error().Err(err).Str("email_id", j.ID).
				Msg("event details worker: sent but could not record it — may send again after the lease")
		}

	case isPermanentEmailErr(err):
		// Unverified sender, suspended account, malformed address: retrying
		// cannot fix it. Left as 'failed' for review.
		log.Error().Err(err).Str("email_id", j.ID).Str("notice_id", j.NoticeID).
			Msg("event details worker: update email permanently rejected")
		if cerr := repository.CloseEventUpdateEmail(pool, opCtx, j.ID, "failed", err.Error()); cerr != nil {
			log.Warn().Err(cerr).Str("email_id", j.ID).Msg("event details worker: could not record failure")
		}

	case j.Attempts >= updateEmailMaxAttempts:
		log.Error().Err(err).Str("email_id", j.ID).Int("attempts", j.Attempts).
			Msg("event details worker: update email out of attempts")
		if cerr := repository.CloseEventUpdateEmail(pool, opCtx, j.ID, "failed", err.Error()); cerr != nil {
			log.Warn().Err(cerr).Str("email_id", j.ID).Msg("event details worker: could not record failure")
		}

	default:
		delay := updateEmailBackoffs[len(updateEmailBackoffs)-1]
		if j.Attempts-1 < len(updateEmailBackoffs) && j.Attempts >= 1 {
			delay = updateEmailBackoffs[j.Attempts-1]
		}
		log.Warn().Err(err).Str("email_id", j.ID).Int("attempts", j.Attempts).Dur("retry_in", delay).
			Msg("event details worker: update email failed — will retry")
		if rerr := repository.RetryEventUpdateEmail(pool, opCtx, j.ID, delay, err.Error()); rerr != nil {
			log.Warn().Err(rerr).Str("email_id", j.ID).Msg("event details worker: could not schedule retry")
		}
	}
}