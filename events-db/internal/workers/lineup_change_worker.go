package workers

import (
	"context"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const (
	// lineupPollInterval — how often to look for changed line-ups.
	lineupPollInterval = 2 * time.Minute

	// lineupSettleWindow — how long a line-up must sit unchanged before its
	// change is announced.
	//
	// An organiser building a line-up adds acts one at a time, saving after
	// each. Without this, eight acts means eight notifications to every ticket
	// holder — which is how an app loses its notification permission. Five
	// minutes collapses an editing session into one message.
	lineupSettleWindow = 5 * time.Minute

	lineupEventsPerCycle = 20
	lineupAudiencePage   = 500

	// lineupMaxRecipients — blast-radius ceiling, not a scaling limit. If a bug
	// ever produces an event with a million ticket holders, this worker should
	// not be what discovers it.
	lineupMaxRecipients = 100_000
)

// StartLineupChangeWorker notifies ticket holders when a live event's line-up
// changes, and the organiser's followers when acts are added or removed.
//
// ── Why this exists ─────────────────────────────────────────────────────────
//
// Phase 4 lets an organiser change a live event's line-up immediately, with no
// admin review. That is right — an act drops out three days out and the
// replacement is announced the same afternoon; a line-up in a review queue is
// wrong on exactly the days most people are looking at it.
//
// But it means an organiser can drop a headliner silently, and somebody who
// paid to see that act finds out by arriving. The answer is not to block the
// change: a correct page nobody was told about still beats a stale one.
// Blocking would give us a stale page AND an uninformed buyer. The answer is to
// tell the people with a stake.
//
// ── Why a worker rather than firing from the save handler ───────────────────
//
// Same reasoning as the announce worker in phase 2, plus one more:
//
//   - the line-up is written by TWO paths — the organiser's PUT and the admin
//     edit-request executor — so a hook would need to live in both
//   - a fan-out on the save request makes the organiser wait
//   - saving eight times while building a line-up would fire eight times
//
// Polling for the STATE ("do the current names differ from what we last
// announced?") covers every write path, present and future, and is naturally
// idempotent: if a cycle dies half way, the snapshot still holds the old names
// and the next cycle recomputes the same diff.
func StartLineupChangeWorker(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().
			Dur("interval", lineupPollInterval).
			Dur("settle", lineupSettleWindow).
			Msg("line-up change worker: started")

		ticker := time.NewTicker(lineupPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runLineupChangeCycle(pool, ctx)
			case <-ctx.Done():
				log.Info().Msg("line-up change worker: stopped")
				return
			}
		}
	}()
}

func runLineupChangeCycle(pool *pgxpool.Pool, ctx context.Context) {
	cycleCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	candidates, err := repository.FindChangedLineups(
		pool, cycleCtx, lineupSettleWindow, lineupEventsPerCycle,
	)
	if err != nil {
		log.Error().Err(err).Msg("line-up worker: candidate query failed")
		return
	}

	for _, c := range candidates {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// An event published after this feature shipped has no snapshot yet.
		// Its line-up was built while it was a draft, so nobody has been told
		// anything about it — announcing the whole thing as "added" would fire
		// a second push moments after phase 2's "new event" announcement for
		// the same event. Record it silently as the baseline instead.
		if c.NeverAnnounced {
			if err := repository.MarkLineupAnnounced(pool, cycleCtx, c.EventID, c.Current); err != nil {
				log.Warn().Err(err).Str("event_id", c.EventID).
					Msg("line-up worker: could not seed baseline snapshot")
			}
			continue
		}

		diff := models.DiffSnapshots(c.Announced, c.Current)
		if !diff.IsMaterial() {
			// Names identical. The organiser moved a stage, shifted a set time,
			// uploaded a photo or reordered the list — all real edits, none of
			// them "the act you paid to see is gone".
			//
			// The snapshot is still refreshed so this event stops being
			// re-examined every cycle for a change that will never be material.
			if err := repository.MarkLineupAnnounced(pool, cycleCtx, c.EventID, c.Current); err != nil {
				log.Warn().Err(err).Str("event_id", c.EventID).
					Msg("line-up worker: could not refresh snapshot")
			}
			continue
		}

		if err := announceLineupChange(pool, cycleCtx, c, diff); err != nil {
			log.Error().Err(err).
				Str("event_id", c.EventID).
				Msg("line-up worker: fan-out failed")
			// Snapshot deliberately NOT advanced. The old names stay recorded,
			// so the next cycle recomputes the same diff and finishes the job.
			continue
		}

		if err := repository.MarkLineupAnnounced(pool, cycleCtx, c.EventID, c.Current); err != nil {
			// Notifications went out but the snapshot did not move, so the next
			// cycle will send them again. Log loudly — this is the one failure
			// mode that produces a duplicate.
			log.Error().Err(err).Str("event_id", c.EventID).
				Msg("line-up worker: notified but could not record snapshot — may re-notify")
		}
	}
}

// announceLineupChange notifies ticket holders, and followers when the change
// is an addition or a removal.
func announceLineupChange(
	pool *pgxpool.Pool,
	ctx context.Context,
	c repository.LineupWatchCandidate,
	diff models.LineupDiff,
) error {
	title := "Line-up update"
	body := diff.NotificationBody(c.EventTitle)

	payload := models.LineupChangePayload{
		EventID:     c.EventID,
		EventSlug:   c.EventSlug,
		EventTitle:  c.EventTitle,
		Added:       diff.Added,
		Removed:     diff.Removed,
		HasRemovals: len(diff.Removed) > 0,
	}

	// The organiser does not need telling about their own edit.
	organiserUserID, _ := repository.GetEventOrganizerUserID(pool, ctx, c.EventID)

	// notified guards against a user who is BOTH a ticket holder and a
	// follower of the organiser getting the same message twice.
	notified := make(map[string]bool, 256)
	if organiserUserID != "" {
		notified[organiserUserID] = true
	}

	sent, failed := 0, 0

	send := func(userID string) {
		if notified[userID] {
			return
		}
		notified[userID] = true
		if err := repository.CreateNotification(
			pool, ctx, userID, models.NotifEventLineupChanged, title, body, payload,
		); err != nil {
			failed++
			return
		}
		sent++
	}

	// ── Ticket holders — always ──────────────────────────────────────────────
	//
	// They paid money against a specific line-up. They are the reason this
	// worker exists, and they are told about ANY material change.
	var cursor *string
	for {
		page, err := repository.ListEventTicketHolderIDs(pool, ctx, c.EventID, cursor, lineupAudiencePage)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		for _, uid := range page {
			send(uid)
		}
		if len(page) < lineupAudiencePage || sent+failed >= lineupMaxRecipients {
			break
		}
		last := page[len(page)-1]
		cursor = &last
	}

	// ── Organiser followers — additions and removals only ────────────────────
	//
	// Followers asked to hear about this organiser's events, not to audit
	// their edits. A new headliner is news worth a push; a corrected stage
	// name is not — and DiffSnapshots has already ruled the latter out.
	//
	// Ticket holders are notified above regardless, so a follower who also
	// holds a ticket has already been reached and send() skips them.
	if organiserUserID != "" {
		organizerID, err := repository.GetOrganizerIDForUser(pool, organiserUserID)
		if err == nil {
			var fCursor *string
			for {
				page, err := repository.ListOrganizerFollowerIDs(
					pool, ctx, organizerID, organiserUserID, fCursor, lineupAudiencePage,
				)
				if err != nil {
					// Followers are a bonus audience. Failing to reach them
					// must not undo the ticket holders who were already told,
					// so this returns nil rather than an error — returning one
					// would keep the snapshot stale and re-notify everybody
					// next cycle.
					log.Warn().Err(err).Str("event_id", c.EventID).
						Msg("line-up worker: follower fan-out failed; ticket holders were notified")
					break
				}
				if len(page) == 0 {
					break
				}
				for _, uid := range page {
					send(uid)
				}
				if len(page) < lineupAudiencePage || sent+failed >= lineupMaxRecipients {
					break
				}
				last := page[len(page)-1]
				fCursor = &last
			}
		}
	}

	log.Info().
		Str("event_id", c.EventID).
		Str("event", c.EventTitle).
		Strs("added", diff.Added).
		Strs("removed", diff.Removed).
		Int("notified", sent).
		Int("failed", failed).
		Msg("line-up worker: change announced")

	return nil
}