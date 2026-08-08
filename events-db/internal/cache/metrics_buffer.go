package cache

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"events-db/internal/repository"

	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Event metric buffer — impressions / views / dwell, bucketed by minute.
//
// Same shape as the existing view counter (view_counter.go): counters land in
// Redis on the hot path and a worker drains them into Postgres every 60s. The
// difference is the key carries the minute, so the drain produces a time series
// instead of a single lifetime total.
//
// Why buffer at all: the homepage records an impression for EVERY card in EVERY
// feed response. That is easily thousands of increments per second at launch
// volume, and none of them can be allowed to add latency to the feed or to
// contend on an events row. HINCRBY into a per-minute hash is one round trip
// and touches no Postgres lock.
//
// Failure posture is the same as the view counter: metrics are not money. If
// Redis is unavailable every function here degrades to a no-op and the caller
// falls back to a direct Postgres upsert (see repository.RecordMetricsDirect).
// A dropped impression is invisible; a 500 on the homepage is not.
// ─────────────────────────────────────────────────────────────────────────────

const (
	// metricKeyPrefix namespaces the buckets so the existing FlushViews SCAN
	// over "views:*" can never collide with them.
	metricKeyPrefix = "evtm:"

	// metricKeyTTL is a safety net, not the flush mechanism — the worker
	// consumes keys with GETDEL/DEL every 60s. If the worker dies, this stops
	// the keyspace growing without bound; the counters in flight are lost,
	// which is the correct trade for a metrics buffer.
	metricKeyTTL = 30 * time.Minute
)

// Field names inside the per-minute hash.
const (
	fieldImpressions  = "imp"
	fieldViews        = "view"
	fieldDwellMs      = "dms"
	fieldDwellSamples = "dn"
)

// The drained bucket type lives in internal/repository, not here: this package
// already imports repository, so declaring it locally and having repository
// consume it would be an import cycle.

// metricKey builds "evtm:<eventID>:<unixMinute>".
//
// The minute is stored as a unix minute count rather than a formatted string:
// it is unambiguous about zone (there isn't one — it's an instant), it is
// cheap to parse back, and it sorts naturally.
func metricKey(eventID string, at time.Time) string {
	return fmt.Sprintf("%s%s:%d", metricKeyPrefix, eventID, at.UTC().Unix()/60)
}

// RecordImpression buffers one feed appearance for an event.
//
// Returns false when Redis is unavailable so the caller can decide whether to
// fall back to Postgres. Feed paths deliberately DON'T fall back — an impression
// is low-value and the feed is the hottest path in the product.
func (c *EventCache) RecordImpression(ctx context.Context, eventID string) bool {
	return c.addMetric(ctx, eventID, fieldImpressions, 1)
}

// RecordImpressions buffers a whole feed response in one pipeline.
//
// The homepage returns ~40 cards across its sections; issuing 40 round trips
// per request would cost more than the query that built the feed. Duplicate ids
// across sections are collapsed by the caller, so an event that appears in both
// "Trending" and "This Weekend" counts once per response — the organiser is
// being told how many people saw it, not how many tiles it occupied.
func (c *EventCache) RecordImpressions(ctx context.Context, eventIDs []string) bool {
	if c.rdb == nil || len(eventIDs) == 0 {
		return false
	}
	now := time.Now()
	pipe := c.rdb.Pipeline()
	for _, id := range eventIDs {
		k := metricKey(id, now)
		pipe.HIncrBy(ctx, k, fieldImpressions, 1)
		pipe.Expire(ctx, k, metricKeyTTL)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Warn().Err(err).Int("events", len(eventIDs)).Msg("impression buffer failed")
		return false
	}
	return true
}

// RecordViewBucket buffers one event-detail open into the time series.
//
// This is ADDITIVE to IncrementView: that one keeps feeding events.views, which
// the trending score and the public event page still read. Both are kept so
// this migration can't change what any existing surface displays.
func (c *EventCache) RecordViewBucket(ctx context.Context, eventID string) bool {
	return c.addMetric(ctx, eventID, fieldViews, 1)
}

// RecordDwell buffers one dwell report (milliseconds spent on the detail
// screen) plus the sample count that makes coverage measurable.
func (c *EventCache) RecordDwell(ctx context.Context, eventID string, ms int64) bool {
	if c.rdb == nil || ms <= 0 {
		return false
	}
	k := metricKey(eventID, time.Now())
	pipe := c.rdb.Pipeline()
	pipe.HIncrBy(ctx, k, fieldDwellMs, ms)
	pipe.HIncrBy(ctx, k, fieldDwellSamples, 1)
	pipe.Expire(ctx, k, metricKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Warn().Err(err).Str("event_id", eventID).Msg("dwell buffer failed")
		return false
	}
	return true
}

func (c *EventCache) addMetric(ctx context.Context, eventID, field string, by int64) bool {
	if c.rdb == nil {
		return false
	}
	k := metricKey(eventID, time.Now())
	pipe := c.rdb.Pipeline()
	pipe.HIncrBy(ctx, k, field, by)
	pipe.Expire(ctx, k, metricKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Warn().Err(err).Str("event_id", eventID).Str("field", field).
			Msg("metric buffer failed")
		return false
	}
	return true
}

// FlushMetrics drains every buffered minute bucket and returns them for the
// caller to upsert into Postgres.
//
// Two properties matter here and both are load-bearing:
//
//  1. SCAN, never KEYS. KEYS blocks Redis for the whole sweep; view_counter.go
//     was already fixed for exactly this and the same rule applies.
//
//  2. The CURRENT minute is deliberately skipped. Draining a minute that is
//     still accumulating would write a partial bucket, and the next drain would
//     write the remainder as a second row — which the upsert then ADDS, so the
//     total stays correct but only because the upsert is additive. Skipping the
//     open minute keeps each bucket a single clean write and makes the
//     "last full minute" the newest point the dashboard will ever show.
func (c *EventCache) FlushMetrics(ctx context.Context) []repository.MetricBucket {
	if c.rdb == nil {
		return nil
	}

	currentMinute := time.Now().UTC().Unix() / 60

	var cursor uint64
	var keys []string
	for {
		batch, next, err := c.rdb.Scan(ctx, cursor, metricKeyPrefix+"*", scanCount).Result()
		if err != nil {
			log.Warn().Err(err).Msg("metric flush: scan error")
			return nil
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(keys) == 0 {
		return nil
	}

	out := make([]repository.MetricBucket, 0, len(keys))
	for _, key := range keys {
		eventID, minute, ok := parseMetricKey(key)
		if !ok {
			continue
		}
		if minute >= currentMinute {
			continue // still filling — leave it for the next pass
		}

		vals, err := c.rdb.HGetAll(ctx, key).Result()
		if err != nil {
			log.Warn().Err(err).Str("key", key).Msg("metric flush: hgetall error")
			continue
		}
		// Consume only after a successful read. If the delete fails the bucket
		// is re-read next pass; the upsert is additive, so re-reading would
		// double-count — hence the delete result is checked and the bucket is
		// dropped if it can't be consumed.
		if err := c.rdb.Del(ctx, key).Err(); err != nil {
			log.Warn().Err(err).Str("key", key).Msg("metric flush: del failed, skipping bucket")
			continue
		}

		b := repository.MetricBucket{
			EventID:      eventID,
			BucketAt:     time.Unix(minute*60, 0).UTC(),
			Impressions:  atoi64(vals[fieldImpressions]),
			Views:        atoi64(vals[fieldViews]),
			DwellMs:      atoi64(vals[fieldDwellMs]),
			DwellSamples: atoi64(vals[fieldDwellSamples]),
		}
		if b.Impressions == 0 && b.Views == 0 && b.DwellMs == 0 && b.DwellSamples == 0 {
			continue
		}
		out = append(out, b)
	}
	return out
}

// parseMetricKey splits "evtm:<uuid>:<minute>" back into its parts.
// Uses LastIndex rather than Split so a UUID containing a colon (it can't, but
// the key format shouldn't depend on that) still parses correctly.
func parseMetricKey(key string) (eventID string, minute int64, ok bool) {
	rest := strings.TrimPrefix(key, metricKeyPrefix)
	i := strings.LastIndex(rest, ":")
	if i <= 0 {
		return "", 0, false
	}
	m, err := strconv.ParseInt(rest[i+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return rest[:i], m, true
}

func atoi64(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}