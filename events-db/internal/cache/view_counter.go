package cache

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

func viewKey(eventID string) string {
	return fmt.Sprintf("views:%s", eventID)
}

// IncrementView atomically increments the Redis view counter for an event.
// Non-blocking — if Redis is unavailable, the call is silently skipped.
func (c *EventCache) IncrementView(ctx context.Context, eventID string) {
	if c.rdb == nil {
		return
	}
	if err := c.rdb.Incr(ctx, viewKey(eventID)).Err(); err != nil {
		log.Warn().Err(err).Str("event_id", eventID).Msg("view counter incr failed")
	}
}

// FlushViews reads all pending view counts from Redis and returns them as a
// map[eventID]count. The caller is responsible for writing the totals to
// Postgres; each key is atomically consumed via GETDEL so counts are not
// double-counted if the server restarts mid-flush.
//
// FIX 1: Previously used KEYS "views:*" which blocks Redis for the entire scan
// duration. Now uses SCAN to iterate the keyspace in non-blocking batches.
func (c *EventCache) FlushViews(ctx context.Context) map[string]int64 {
	var cursor uint64
	var allKeys []string

	for {
		keys, nextCursor, err := c.rdb.Scan(ctx, cursor, "views:*", scanCount).Result()
		if err != nil {
			log.Warn().Err(err).Msg("view flush: scan error")
			return nil
		}
		allKeys = append(allKeys, keys...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(allKeys) == 0 {
		return nil
	}

	result := make(map[string]int64, len(allKeys))
	for _, key := range allKeys {
		count, err := c.rdb.GetDel(ctx, key).Int64()
		if err != nil {
			log.Warn().Err(err).Str("key", key).Msg("view flush get error")
			continue
		}
		eventID := strings.TrimPrefix(key, "views:")
		result[eventID] = count
	}
	return result
}