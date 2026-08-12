package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Live presence
//
// "Active users" previously meant "confirmed a booking in the last 90 days",
// which is a retention cohort — a useful number, but not what a tile labelled
// "Active users" is read as. This is the live one: who is using the product
// right now.
//
// WHY A SORTED SET AND NOT A COUNTER
//
// A counter can only answer one question and cannot be corrected. A sorted set
// keyed by user id with the timestamp as score answers EVERY window from one
// structure — active in the last minute, five minutes, hour, day — because the
// window is just a score range at read time. It also deduplicates for free: a
// user hitting forty endpoints in a minute is one member, not forty.
//
// WHY REDIS AND NOT A last_seen_at COLUMN
//
// A column means a database write on every authenticated request. At the read
// side it also means a full scan or an index on a column that changes
// constantly, which is the worst thing to put an index on. ZADD is O(log n)
// against a set whose size is the number of people who used the product today.
//
// COST OF BEING WRONG
//
// Presence is a metric, not a control. Every function here degrades to a no-op
// or a zero when Redis is unavailable — a missing number on a dashboard is
// nothing, a 500 on every authenticated request because the presence write
// failed would be the whole product.
// ─────────────────────────────────────────────────────────────────────────────

const (
	presenceKey = "presence:users"

	// Members older than this are pruned. 25 hours rather than 24 so a
	// "last 24 hours" query always has a full window available — pruning at
	// exactly 24h would race the read and lop off the oldest bucket.
	presenceRetention = 25 * time.Hour
)

// TouchPresence records that a user was seen just now.
//
// Fire-and-forget: called on the tail of every authenticated request, so it
// must never block the response or surface an error. The caller does not check
// the result and there is nothing useful it could do with one.
func (c *EventCache) TouchPresence(ctx context.Context, userID string) {
	if c.rdb == nil || userID == "" {
		return
	}
	now := float64(time.Now().Unix())
	// ZADD overwrites the score for an existing member, so a returning user
	// moves rather than duplicating. No read-modify-write, no race.
	if err := c.rdb.ZAdd(ctx, presenceKey, redis.Z{
		Score:  now,
		Member: userID,
	}).Err(); err != nil {
		log.Debug().Err(err).Msg("presence: touch failed")
		return
	}

	// Trim opportunistically rather than on a timer. Doing it here means the
	// set is bounded by activity — a quiet night leaves it untrimmed, which
	// costs nothing because it is also small. A dedicated cleanup worker would
	// be a second moving part for a set that self-limits.
	//
	// 1-in-100 so the cost is amortised instead of paid on every request.
	if time.Now().UnixNano()%100 == 0 {
		cutoff := float64(time.Now().Add(-presenceRetention).Unix())
		c.rdb.ZRemRangeByScore(ctx, presenceKey, "-inf", formatScore(cutoff))
	}
}

// CountActiveSince returns how many distinct users were seen within the window.
//
// Returns 0 when Redis is unavailable. A zero on a dashboard reads as "nobody
// right now", which is wrong but harmless; the alternative — surfacing an error
// into a stats endpoint that also carries revenue — would blank figures that
// are fine.
func (c *EventCache) CountActiveSince(ctx context.Context, window time.Duration) int64 {
	if c.rdb == nil {
		return 0
	}
	min := formatScore(float64(time.Now().Add(-window).Unix()))
	n, err := c.rdb.ZCount(ctx, presenceKey, min, "+inf").Result()
	if err != nil {
		log.Debug().Err(err).Msg("presence: count failed")
		return 0
	}
	return n
}

// ActiveUserIDs returns the users seen within the window, most recent first.
// Bounded by `limit` because the caller is rendering a list, not a census.
func (c *EventCache) ActiveUserIDs(ctx context.Context, window time.Duration, limit int64) []string {
	if c.rdb == nil {
		return nil
	}
	min := formatScore(float64(time.Now().Add(-window).Unix()))
	ids, err := c.rdb.ZRevRangeByScore(ctx, presenceKey, &redis.ZRangeBy{
		Min:   min,
		Max:   "+inf",
		Count: limit,
	}).Result()
	if err != nil {
		log.Debug().Err(err).Msg("presence: range failed")
		return nil
	}
	return ids
}

// formatScore renders a unix second as a Redis score bound.
//
// Written by hand rather than via strconv on a float so the value can never
// come out in scientific notation — Redis rejects "1.7e+09" as a range bound,
// and a formatting bug here would silently return zero active users forever.
func formatScore(v float64) string {
	return itoa(int64(v))
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}