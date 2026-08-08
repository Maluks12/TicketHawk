package workers

import (
	"context"

	"events-db/internal/cache"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// flushMetricBuckets drains the Redis minute buckets (impressions, views,
// dwell) into event_metric_buckets.
//
// WIRING — one line to add in expiry.go. Find the existing view tick:
//
//	case <-viewTicker.C:
//	    flushViewCounts(pool, ec, ctx)
//	    flushMetricBuckets(pool, ec, ctx)   // <- add this
//
// The same 60s ticker is reused deliberately rather than adding a second one:
// both drains talk to the same Redis and the same Postgres, and interleaving
// them on one tick keeps the load predictable instead of having two timers
// drift into each other.
//
// This runs ALONGSIDE flushViewCounts, not instead of it. flushViewCounts keeps
// feeding events.views, which the trending score and the public event page
// still read; this adds the time dimension the dashboard needs. Both counters
// are incremented from the same request, so they should agree — if they ever
// diverge, events.views is the one every existing surface trusts.
func flushMetricBuckets(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context) {
	if ec == nil {
		return
	}
	buckets := ec.FlushMetrics(ctx)
	if len(buckets) == 0 {
		return
	}

	if err := repository.UpsertMetricBuckets(pool, buckets); err != nil {
		// The buckets are already consumed from Redis at this point, so a
		// failure here loses them. That is the accepted trade for a metrics
		// pipeline: the alternative is holding them in Redis until a write
		// succeeds, which turns a Postgres outage into unbounded memory growth
		// on the cache that also serves the feed.
		log.Error().Err(err).Int("buckets", len(buckets)).
			Msg("metric flush: upsert failed, buckets dropped")
		return
	}

	log.Debug().Int("buckets", len(buckets)).Msg("metric flush: complete")
}