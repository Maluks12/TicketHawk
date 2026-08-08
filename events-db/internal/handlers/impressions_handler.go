package handlers

import (
	"context"

	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
)

// ─────────────────────────────────────────────────────────────────────────────
// Impression recording
//
// An impression is "this event's card was in a response someone asked for".
// Counting it SERVER-SIDE is a deliberate choice with a real trade-off:
//
//	+ It works today across every client — EventsApp, web-desktop, web-mobile —
//	  with no app release, no beacon, no viewability library. Organisers get
//	  reach numbers the moment this deploys.
//	+ It can't be inflated or blocked by the client.
//	- It counts cards that were RETURNED, not cards that were SEEN. A card below
//	  the fold on a feed nobody scrolled still counts.
//
// So this measures REACH — "how many feeds you appeared in" — which is exactly
// how the dashboard labels it. It is not a viewability metric and the UI does
// not claim it is.
//
// Deduplication per response matters: the homepage can legitimately show the
// same event in "Trending Now" and "This Weekend". That is one person seeing
// the event once, not twice, and counting it twice would inflate reach for
// precisely the events that are doing well.
// ─────────────────────────────────────────────────────────────────────────────

// recordFeedImpressions buffers one impression per DISTINCT event in a feed
// response.
//
// Fire-and-forget by design. This runs on the hottest path in the product and
// an analytics counter must never add latency to it or fail it — if Redis is
// down the impressions are simply lost, which nobody will notice, whereas a
// slow or erroring homepage is the whole product.
func recordFeedImpressions(ctx context.Context, ec *cache.EventCache, events []models.Event) {
	if ec == nil || len(events) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(events))
	ids := make([]string, 0, len(events))
	for _, e := range events {
		if e.ID == "" {
			continue
		}
		if _, dup := seen[e.ID]; dup {
			continue
		}
		seen[e.ID] = struct{}{}
		ids = append(ids, e.ID)
	}
	if len(ids) == 0 {
		return
	}
	ec.RecordImpressions(ctx, ids)
}

// recordSectionImpressions flattens a set of home sections and records one
// impression per distinct event across ALL of them.
func recordSectionImpressions(ctx context.Context, ec *cache.EventCache, sections []repository.HomeSection) {
	if ec == nil || len(sections) == 0 {
		return
	}
	total := 0
	for _, s := range sections {
		total += len(s.Events)
	}
	flat := make([]models.Event, 0, total)
	for _, s := range sections {
		flat = append(flat, s.Events...)
	}
	recordFeedImpressions(ctx, ec, flat)
}