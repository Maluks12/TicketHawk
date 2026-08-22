package workers

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Tuning ───────────────────────────────────────────────────────────────────

const (
	// announcePollInterval is how often the worker looks for newly live events.
	// A minute is well inside "felt immediate" for an announcement while
	// costing one index probe per cycle on a partial index that is empty
	// almost always.
	announcePollInterval = 1 * time.Minute

	// announceGrace is how long an event must have been live before its
	// announcement goes out.
	//
	// Approval is reversible in the first minutes — an admin approves the wrong
	// row, notices, and takes it down. Once a push has landed on tens of
	// thousands of phones there is no taking it back, so the fan-out waits long
	// enough for that mistake to be caught. Two minutes costs the organiser
	// nothing and buys a real undo window.
	announceGrace = 2 * time.Minute

	// announceEventsPerCycle caps how many events one cycle claims. Bounds the
	// worst case where a backlog is released at once — an admin clearing a
	// review queue of forty events in one sitting.
	announceEventsPerCycle = 20

	// announceFollowerPage is the keyset page size for the fan-out.
	announceFollowerPage = 500

	// announceMaxFollowers is a hard ceiling per event.
	//
	// Not a scaling limit — it is a blast-radius limit. If a bug or an abusive
	// account ever produces an organiser with a million followers, this worker
	// should not be the thing that discovers it by writing a million rows and
	// firing a million pushes. Hitting the ceiling logs loudly and stops.
	announceMaxFollowers = 100_000
)

// StartOrganizerAnnounceWorker notifies an organiser's followers when one of
// their events goes live publicly.
//
// ── Why this is a worker and not a hook on the publish path ─────────────────
//
// An event reaches 'published' by THREE routes today: AdminApproveEvent from
// the review queue, ApproveEventWithFeature, and AdminPromoteEvent, which
// publishes as a side effect of promoting. Hooking any one of them announces
// two thirds of events; hooking all three is three places to keep in step, and
// the fourth path will be added by someone who has never read this comment.
//
// Polling for the STATE ("live, public, unannounced") instead of reacting to
// the TRANSITION covers every route that exists, every route added later, and
// direct SQL run by an admin during an incident. It also takes the fan-out off
// the admin's request — approving an event should not block on writing 50 000
// notification rows — and it retries by itself after a restart, because the
// state it looks for is still true.
//
// The cost is up to announceGrace + one poll interval of latency, which for an
// announcement nobody is waiting on is not a cost at all.
func StartOrganizerAnnounceWorker(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().
			Dur("interval", announcePollInterval).
			Dur("grace", announceGrace).
			Msg("organiser announce worker: started")

		ticker := time.NewTicker(announcePollInterval)
		defer ticker.Stop()

		// Deliberately NO immediate first run, unlike the reminder worker.
		//
		// On a restart during a deploy, every container starts within seconds
		// of the others. An immediate cycle in each means they all contend for
		// the same claim in the same instant — SKIP LOCKED handles that
		// correctly, but there is nothing to gain: the grace period means
		// anything worth announcing has been waiting minutes already and can
		// wait one more.
		for {
			select {
			case <-ticker.C:
				runAnnounceCycle(pool, ctx)
			case <-ctx.Done():
				log.Info().Msg("organiser announce worker: stopped")
				return
			}
		}
	}()
}

func runAnnounceCycle(pool *pgxpool.Pool, ctx context.Context) {
	cycleCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	claimed, err := repository.ClaimPendingAnnouncements(
		pool, cycleCtx, announceGrace, announceEventsPerCycle,
	)
	if err != nil {
		log.Error().Err(err).Msg("announce worker: claim failed")
		return
	}
	if len(claimed) == 0 {
		return
	}

	log.Info().Int("events", len(claimed)).Msg("announce worker: claimed events")

	for _, a := range claimed {
		select {
		case <-ctx.Done():
			// Shutting down mid-batch. The remaining events stay claimed and
			// will NOT be retried — see the note on ReleaseAnnouncementClaim
			// for why a partial fan-out is never replayed.
			log.Warn().
				Str("event_id", a.EventID).
				Msg("announce worker: shutdown mid-batch, remaining events skipped")
			return
		default:
		}

		if err := announceEvent(pool, cycleCtx, a); err != nil {
			log.Error().Err(err).
				Str("event_id", a.EventID).
				Str("event_title", a.EventTitle).
				Msg("announce worker: fan-out failed")
		}
	}
}

// announceEvent writes one notification per follower.
//
// Notifications go out one at a time through repository.CreateNotification
// rather than as a bulk INSERT. That is a deliberate trade: the bulk form would
// be one round-trip instead of N, but CreateNotification is what fires the push
// dispatcher, and it fires it only AFTER its own row write succeeds — so a
// follower never gets a phone buzz for a notification that is missing from
// their feed. Reimplementing that ordering around a bulk insert to save a few
// hundred milliseconds on a background job is not a trade worth making.
//
// This is the same shape the sell-out worker already uses.
func announceEvent(pool *pgxpool.Pool, ctx context.Context, a repository.PendingAnnouncement) error {
	title := fmt.Sprintf("%s just announced a new event", a.OrganizerName)
	body := a.EventTitle

	cover := ""
	if a.CoverImageURL != nil {
		cover = *a.CoverImageURL
	}

	payload := models.NewEventAnnouncement{
		EventID:       a.EventID,
		EventSlug:     a.EventSlug,
		EventTitle:    a.EventTitle,
		OrganizerID:   a.OrganizerID,
		OrganizerSlug: a.OrganizerSlug,
		OrganizerName: a.OrganizerName,
		CoverImageURL: cover,
	}

	var (
		cursor *string
		sent   int
		failed int
	)

	for {
		page, err := repository.ListOrganizerFollowerIDs(
			pool, ctx, a.OrganizerID, a.OrganizerUserID, cursor, announceFollowerPage,
		)
		if err != nil {
			// Could not even read the follower list. Nothing was sent for this
			// event yet if this is the first page, so releasing the claim is
			// safe and the next cycle retries cleanly.
			if sent == 0 {
				if rErr := repository.ReleaseAnnouncementClaim(pool, ctx, a.EventID); rErr != nil {
					log.Error().Err(rErr).Str("event_id", a.EventID).
						Msg("announce worker: could not release claim; event will not be announced")
				}
			}
			return fmt.Errorf("list followers: %w", err)
		}
		if len(page) == 0 {
			break
		}

		for _, userID := range page {
			if err := repository.CreateNotification(
				pool, ctx, userID, models.NotifOrganizerNewEvent, title, body, payload,
			); err != nil {
				// One follower failing must not stop the rest — a single bad
				// row (deleted user racing the read, for instance) should not
				// cost the other 40 000 people their announcement.
				failed++
				continue
			}
			sent++
		}

		if len(page) < announceFollowerPage {
			break
		}
		if sent+failed >= announceMaxFollowers {
			log.Error().
				Str("event_id", a.EventID).
				Str("organizer_id", a.OrganizerID).
				Int("ceiling", announceMaxFollowers).
				Msg("announce worker: follower ceiling hit, fan-out truncated — investigate")
			break
		}

		last := page[len(page)-1]
		cursor = &last
	}

	log.Info().
		Str("event_id", a.EventID).
		Str("organizer", a.OrganizerName).
		Int("notified", sent).
		Int("failed", failed).
		Msg("announce worker: fan-out complete")

	return nil
}