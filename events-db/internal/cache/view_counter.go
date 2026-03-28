package cache
 
import (
	"context"
	"fmt"
	"strings"
 
	//"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)
 
func viewKey(eventID string) string {
	return fmt.Sprintf("views:%s", eventID)
}
 
// IncrementView atomically increments the Redis view counter for an event.
// Non-blocking — if Redis is unavailable, the call is silently skipped.
func (c *EventCache) IncrementView(ctx context.Context, eventID string) {
	if c.rdb == nil { return }
	if err := c.rdb.Incr(ctx, viewKey(eventID)).Err(); err != nil {
		log.Warn().Err(err).Str("event_id", eventID).Msg("view counter incr failed")
	}
}
 
// FlushViews reads all pending view counts from Redis and returns them.
// The caller is responsible for writing to Postgres and then the keys are deleted.
// Returns map[eventID]count.
func (c *EventCache) FlushViews(ctx context.Context) map[string]int64 {
	keys, err := c.rdb.Keys(ctx, "views:*").Result()
	if err != nil || len(keys) == 0 {
		return nil
	}
	result := make(map[string]int64, len(keys))
	for _, key := range keys {
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
