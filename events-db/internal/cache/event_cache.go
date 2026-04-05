package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// ── TTLs ─────────────────────────────────────────────────────────────────────
const (
	TTLHome          = 60 * time.Second
	TTLTrending      = 60 * time.Second
	TTLFeatured      = 120 * time.Second
	TTLHappeningSoon = 60 * time.Second
	TTLRecentlyAdded = 30 * time.Second
	TTLThisWeekend   = 300 * time.Second
	TTLCategory      = 60 * time.Second
	TTLMerchPreview  = 300 * time.Second
	TTLMerchFull     = 300 * time.Second
	TTLMerchItem     = 300 * time.Second

	// scanCount is the COUNT hint passed to Redis SCAN per iteration.
	// Redis treats this as a hint, not a guarantee, but 100 is a reasonable
	// batch size that avoids both tiny round-trips and large blocking scans.
	scanCount = 100
)

type EventCache struct {
	rdb *redis.Client
}

func NewEventCache(rdb *redis.Client) *EventCache {
	return &EventCache{rdb: rdb}
}

// ── Key builders ──────────────────────────────────────────────────────────────

func keyHome() string                   { return "home:sections" }
func keyTrending(limit int) string      { return fmt.Sprintf("events:trending:l%d", limit) }
func keyFeatured(limit int) string      { return fmt.Sprintf("events:featured:l%d", limit) }
func keyHappeningSoon(limit int) string { return fmt.Sprintf("events:happening_soon:l%d", limit) }
func keyRecentlyAdded(limit int) string { return fmt.Sprintf("events:recently_added:l%d", limit) }
func keyThisWeekend(limit int) string   { return fmt.Sprintf("events:this_weekend:l%d", limit) }
func keyCategory(categoryID, limit int) string {
	return fmt.Sprintf("events:category:%d:l%d", categoryID, limit)
}
func keyMerchPreview(eventID string) string { return fmt.Sprintf("merch:%s:preview", eventID) }
func keyMerchFull(eventID string) string    { return fmt.Sprintf("merch:%s:full", eventID) }
func keyMerchItem(merchID string) string    { return fmt.Sprintf("merch:item:%s", merchID) }

// ── Generic helpers ───────────────────────────────────────────────────────────

func (c *EventCache) get(ctx context.Context, key string, dest any) bool {
	val, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			log.Warn().Err(err).Str("key", key).Msg("cache get error")
		}
		return false
	}
	if err := json.Unmarshal(val, dest); err != nil {
		log.Warn().Err(err).Str("key", key).Msg("cache unmarshal error")
		return false
	}
	return true
}

func (c *EventCache) set(ctx context.Context, key string, val any, ttl time.Duration) {
	data, err := json.Marshal(val)
	if err != nil {
		log.Warn().Err(err).Str("key", key).Msg("cache marshal error")
		return
	}
	if err := c.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		log.Warn().Err(err).Str("key", key).Msg("cache set error")
	}
}

func (c *EventCache) del(ctx context.Context, keys ...string) {
	if len(keys) == 0 {
		return
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		log.Warn().Err(err).Strs("keys", keys).Msg("cache delete error")
	}
}

// FIX 1: delPattern uses SCAN instead of KEYS so it never blocks Redis.
//
// KEYS runs in O(N) against the entire keyspace in a single synchronous call —
// on a large dataset this stalls every other Redis client for the duration.
// SCAN iterates in O(1) batches and interleaves safely with other commands.
func (c *EventCache) delPattern(ctx context.Context, pattern string) {
	var cursor uint64
	var toDelete []string

	for {
		keys, nextCursor, err := c.rdb.Scan(ctx, cursor, pattern, scanCount).Result()
		if err != nil {
			log.Warn().Err(err).Str("pattern", pattern).Msg("cache scan error")
			return
		}
		toDelete = append(toDelete, keys...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(toDelete) == 0 {
		return
	}

	// Pipeline the DEL calls in one round-trip to avoid N separate commands.
	pipe := c.rdb.Pipeline()
	for _, k := range toDelete {
		pipe.Del(ctx, k)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Warn().Err(err).Str("pattern", pattern).Msg("cache pipeline del error")
	}
}

// ── EventPage — shared type for paginated event lists ─────────────────────────

type EventPage struct {
	Events     []models.Event `json:"events"`
	NextCursor *utils.Cursor  `json:"next_cursor"`
}

// ── Home ──────────────────────────────────────────────────────────────────────

func (c *EventCache) GetHome(ctx context.Context) (*repository.HomeSectionsResult, bool) {
	var result repository.HomeSectionsResult
	return &result, c.get(ctx, keyHome(), &result)
}

func (c *EventCache) SetHome(ctx context.Context, result *repository.HomeSectionsResult) {
	c.set(ctx, keyHome(), result, TTLHome)
}

// ── Event lists (first page only) ────────────────────────────────────────────

func (c *EventCache) GetTrending(ctx context.Context, limit int) (*EventPage, bool) {
	var p EventPage
	return &p, c.get(ctx, keyTrending(limit), &p)
}
func (c *EventCache) SetTrending(ctx context.Context, limit int, p EventPage) {
	c.set(ctx, keyTrending(limit), p, TTLTrending)
}

func (c *EventCache) GetFeatured(ctx context.Context, limit int) (*EventPage, bool) {
	var p EventPage
	return &p, c.get(ctx, keyFeatured(limit), &p)
}
func (c *EventCache) SetFeatured(ctx context.Context, limit int, p EventPage) {
	c.set(ctx, keyFeatured(limit), p, TTLFeatured)
}

func (c *EventCache) GetHappeningSoon(ctx context.Context, limit int) (*EventPage, bool) {
	var p EventPage
	return &p, c.get(ctx, keyHappeningSoon(limit), &p)
}
func (c *EventCache) SetHappeningSoon(ctx context.Context, limit int, p EventPage) {
	c.set(ctx, keyHappeningSoon(limit), p, TTLHappeningSoon)
}

func (c *EventCache) GetRecentlyAdded(ctx context.Context, limit int) (*EventPage, bool) {
	var p EventPage
	return &p, c.get(ctx, keyRecentlyAdded(limit), &p)
}
func (c *EventCache) SetRecentlyAdded(ctx context.Context, limit int, p EventPage) {
	c.set(ctx, keyRecentlyAdded(limit), p, TTLRecentlyAdded)
}

func (c *EventCache) GetThisWeekend(ctx context.Context, limit int) (*EventPage, bool) {
	var p EventPage
	return &p, c.get(ctx, keyThisWeekend(limit), &p)
}
func (c *EventCache) SetThisWeekend(ctx context.Context, limit int, p EventPage) {
	c.set(ctx, keyThisWeekend(limit), p, TTLThisWeekend)
}

func (c *EventCache) GetCategory(ctx context.Context, categoryID, limit int) (*EventPage, bool) {
	var p EventPage
	return &p, c.get(ctx, keyCategory(categoryID, limit), &p)
}
func (c *EventCache) SetCategory(ctx context.Context, categoryID, limit int, p EventPage) {
	c.set(ctx, keyCategory(categoryID, limit), p, TTLCategory)
}

// ── Merch lists ───────────────────────────────────────────────────────────────

func (c *EventCache) GetMerchPreview(ctx context.Context, eventID string) ([]models.Merchandise, bool) {
	var items []models.Merchandise
	return items, c.get(ctx, keyMerchPreview(eventID), &items)
}
func (c *EventCache) SetMerchPreview(ctx context.Context, eventID string, items []models.Merchandise) {
	c.set(ctx, keyMerchPreview(eventID), items, TTLMerchPreview)
}

func (c *EventCache) GetMerchFull(ctx context.Context, eventID string) ([]models.Merchandise, bool) {
	var items []models.Merchandise
	return items, c.get(ctx, keyMerchFull(eventID), &items)
}
func (c *EventCache) SetMerchFull(ctx context.Context, eventID string, items []models.Merchandise) {
	c.set(ctx, keyMerchFull(eventID), items, TTLMerchFull)
}

// ── Merch single item ─────────────────────────────────────────────────────────

func (c *EventCache) GetMerchItem(ctx context.Context, merchID string) (*models.MerchandiseDetail, bool) {
	var item models.MerchandiseDetail
	return &item, c.get(ctx, keyMerchItem(merchID), &item)
}
func (c *EventCache) SetMerchItem(ctx context.Context, merchID string, item *models.MerchandiseDetail) {
	c.set(ctx, keyMerchItem(merchID), item, TTLMerchItem)
}

// ── Bust helpers ──────────────────────────────────────────────────────────────

// FIX 2: Bust helpers now use delPattern with wildcards instead of hardcoding
// specific limit values (previously only l10 and l20 were deleted, so a client
// that requested limit=30 would keep receiving a stale cached page). delPattern
// is now safe to call frequently because it uses SCAN, not KEYS.

// BustOnPublish — call when a new event is published.
func (c *EventCache) BustOnPublish(ctx context.Context) {
	c.del(ctx, keyHome())
	c.delPattern(ctx, "events:trending:*")
	c.delPattern(ctx, "events:happening_soon:*")
	c.delPattern(ctx, "events:recently_added:*")
	c.delPattern(ctx, "events:this_weekend:*")
}

// BustOnFeaturedChange — call when is_featured changes.
func (c *EventCache) BustOnFeaturedChange(ctx context.Context) {
	c.del(ctx, keyHome())
	c.delPattern(ctx, "events:featured:*")
}

// BustOnTicketSale — call when tickets_sold changes (booking confirmed).
func (c *EventCache) BustOnTicketSale(ctx context.Context) {
	c.del(ctx, keyHome())
	c.delPattern(ctx, "events:trending:*")
}

// BustOnEventUpdate — call when an event is updated (catches all cases).
func (c *EventCache) BustOnEventUpdate(ctx context.Context) {
	c.del(ctx, keyHome())
	c.delPattern(ctx, "events:*")
}

// BustMerch — call when merch or a variant is created/updated for an event.
// Pass optional merchIDs to also bust individual item caches.
func (c *EventCache) BustMerch(ctx context.Context, eventID string, merchIDs ...string) {
	keys := []string{
		keyMerchPreview(eventID),
		keyMerchFull(eventID),
	}
	for _, id := range merchIDs {
		keys = append(keys, keyMerchItem(id))
	}
	c.del(ctx, keys...)
}