package repository

import (
	"context"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// hydrateEventDetail attaches the organiser block, the Information block and
// the line-up to a single event.
//
// ── Why this is BEST-EFFORT and returns no error ────────────────────────────
//
// Every one of these is decoration on a page whose primary job is selling a
// ticket. If the line-up query fails, the right outcome is an event page
// without a line-up — not a 500 that stops someone buying. The caller has
// already got the event; refusing to return it because a secondary query
// stumbled would turn a cosmetic fault into an outage on the most valuable page
// in the product.
//
// Failures are logged at WARN with the event id, so a systematic problem is
// visible in the logs rather than silently degrading every page.
//
// ── Why three round-trips and not one big join ──────────────────────────────
//
// A join across organisers, event_info_items and event_lineup would multiply
// rows — one event with 4 info items and 12 line-up entries produces 48 rows to
// deduplicate in Go. Three indexed lookups against primary and foreign keys are
// both faster and far easier to read. They also fail independently, which is
// what makes the best-effort behaviour above possible at all.
//
// ── Why the detail path is not cached ───────────────────────────────────────
//
// Worth stating explicitly because it is the obvious next question. EventCache
// caches COLLECTIONS — home, trending, featured, category, filter, merch — and
// has no per-event detail entry; GetEventBySlugHandler reads through to the
// database on every request and uses the cache only to top up the view counter.
//
// So there is nothing to invalidate when an organiser changes their logo, their
// socials, an event's parking note or its line-up: the next request already
// sees the new value. That is why no cache-invalidation fan-out was built for
// organiser profile writes. If a per-event detail cache is ever added, THAT is
// the moment to add one — and this comment is the reminder.
func hydrateEventDetail(pool *pgxpool.Pool, event *models.Event) {
	if event == nil || event.ID == "" {
		return
	}

	// ── Organiser ────────────────────────────────────────────────────────────
	//
	// Gated exactly as the standalone organiser page is: kyc_level >= 2, page
	// not disabled, account in good standing. An organiser who has switched
	// their public page off should not have their name, logo and socials
	// appear on their event pages instead — the switch would mean nothing.
	//
	// When the gate blocks them the field is simply absent, and the clients'
	// existing fallback chain (organiser → organizer → hosted_by → venue_name)
	// lands on the venue name, which is what the page shows today.
	if event.OrganizerID != "" {
		if org, err := GetPublicOrganizerProfile(pool, event.OrganizerID); err == nil {
			event.Organizer = org
		} else if err != ErrOrganizerProfileNotFound {
			log.Warn().Err(err).
				Str("event_id", event.ID).
				Str("organizer_id", event.OrganizerID).
				Msg("event detail: organiser block failed to load")
		}
	}

	// ── Information ──────────────────────────────────────────────────────────
	if items, err := GetEventInfoItems(pool, event.ID); err != nil {
		log.Warn().Err(err).Str("event_id", event.ID).
			Msg("event detail: information block failed to load")
	} else if len(items) > 0 {
		event.InfoItems = items
	}

	// ── Line-up ──────────────────────────────────────────────────────────────
	//
	// The legacy Performers/Teams arrays are left untouched alongside this.
	// They are populated from the same rows on every save (see
	// ReplaceEventLineup's dual write) and shipped mobile builds read them
	// directly, so removing them from the payload would empty the line-up
	// section on every phone that has not updated.
	if entries, err := GetEventLineup(pool, event.ID); err != nil {
		log.Warn().Err(err).Str("event_id", event.ID).
			Msg("event detail: line-up failed to load")
	} else if len(entries) > 0 {
		event.Lineup = entries
	}
}

// HydrateEventDetail is the exported form, for callers outside this package
// that assemble an event by other means — the dashboard preview being the one
// that exists today.
func HydrateEventDetail(pool *pgxpool.Pool, event *models.Event) {
	hydrateEventDetail(pool, event)
}

// ── Organiser profile change: what needs busting ────────────────────────────

// NoteOrganizerProfileChanged is a deliberate no-op with a comment attached.
//
// It exists because "an organiser changed their logo, so every cached event of
// theirs is stale" is the correct instinct and the first thing anyone will
// reach for. Today it is unnecessary: the detail path is uncached (see above),
// and the collection caches carry event cards, which do not include the
// organiser block.
//
// Rather than leave that reasoning in a commit message nobody will find, it
// lives here — so the next person to add a per-event cache has a function to
// fill in and a note explaining what to fill it with, instead of shipping a
// cache that serves a deleted logo for its TTL.
//
// Called from the organiser profile write paths for exactly that reason.
func NoteOrganizerProfileChanged(ctx context.Context, organizerID string) {
	_ = ctx
	_ = organizerID
	// Intentionally empty. See the doc comment.
}