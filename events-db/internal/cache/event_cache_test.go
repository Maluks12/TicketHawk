package cache_test

// =============================================================================
// EVENT CACHE TESTS
// =============================================================================
//
// Tests covered:
//   Home:
//     - Cache miss returns false
//     - SetHome stores data, GetHome retrieves it correctly
//     - BustOnPublish deletes home key
//
//   Event lists (trending, featured, happening_soon, recently_added,
//                this_weekend, category):
//     - Miss returns false
//     - Set then Get round-trips correctly
//     - Different limits produce independent cache keys
//     - BustOnPublish clears the right keys
//     - BustOnFeaturedChange only clears featured + home
//     - BustOnTicketSale only clears trending + home
//     - BustOnEventUpdate clears home and all events:* keys
//
//   Merch:
//     - GetMerchPreview / SetMerchPreview round-trip
//     - GetMerchFull / SetMerchFull round-trip
//     - GetMerchItem / SetMerchItem round-trip with stock summary
//     - BustMerch with no IDs clears preview + full
//     - BustMerch with a merchID also clears the item key
//     - Different eventIDs have independent cache keys
//
//   Redis unavailability:
//     - All get methods return false gracefully when Redis is down
//     - All set methods are no-ops when Redis is down
//
// Prerequisites:
//   go get github.com/alicebob/miniredis/v2
//
// Run:
//   go test ./internal/cache/... -v
// =============================================================================

import (
	"context"
	"fmt"
	"testing"
	"time"

	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// newTestCache starts a miniredis server and returns an EventCache wired to it,
// plus a cleanup function that stops the server.
func newTestCache(t *testing.T) (*cache.EventCache, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err, "failed to start miniredis")

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ec := cache.NewEventCache(rdb)
	return ec, mr
}

// sampleEvents returns a slice of minimal Event structs for seeding the cache.
func sampleEvents(n int) []models.Event {
	events := make([]models.Event, n)
	for i := 0; i < n; i++ {
		events[i] = models.Event{
			ID:    fmt.Sprintf("event-id-%d", i+1),
			Title: fmt.Sprintf("Event %d", i+1),
		}
	}
	return events
}

// sampleMerch returns a minimal Merchandise slice.
func sampleMerch(n int) []models.Merchandise {
	items := make([]models.Merchandise, n)
	for i := 0; i < n; i++ {
		items[i] = models.Merchandise{
			ID:        fmt.Sprintf("merch-id-%d", i+1),
			Name:      fmt.Sprintf("Item %d", i+1),
			BasePrice: float64((i + 1) * 100),
			IsActive:  true,
		}
	}
	return items
}

// sampleMerchDetail returns a MerchandiseDetail with variants and stock summary.
func sampleMerchDetail(id string) *models.MerchandiseDetail {
	d := &models.MerchandiseDetail{
		Merchandise: models.Merchandise{
			ID:        id,
			Name:      "Test Kit",
			BasePrice: 1200.00,
			IsActive:  true,
		},
	}
	d.Variants = []models.MerchandiseVariant{
		{ID: "var-1", MerchandiseID: id, SKU: "SKU-S", StockQuantity: 5},
		{ID: "var-2", MerchandiseID: id, SKU: "SKU-M", StockQuantity: 10},
		{ID: "var-3", MerchandiseID: id, SKU: "SKU-L", StockQuantity: 0},
	}
	d.ComputeStockSummary()
	return d
}

// sampleHomeSections returns a minimal HomeSectionsResult.
func sampleHomeSections() *repository.HomeSectionsResult {
	return &repository.HomeSectionsResult{
		Sections: []repository.HomeSection{
			{ID: "trending", Title: "Trending Now", Events: sampleEvents(3)},
			{ID: "featured", Title: "Featured Events", Events: sampleEvents(2)},
		},
	}
}

// ─── Home ──────────────────────────────────────────────────────────────────────

func TestCacheHome(t *testing.T) {
	ctx := context.Background()

	t.Run("miss returns false on empty cache", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		_, ok := ec.GetHome(ctx)
		assert.False(t, ok)
	})

	t.Run("SetHome then GetHome returns correct data", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		in := sampleHomeSections()
		ec.SetHome(ctx, in)

		out, ok := ec.GetHome(ctx)
		require.True(t, ok)
		require.Len(t, out.Sections, 2)
		assert.Equal(t, "trending", out.Sections[0].ID)
		assert.Equal(t, "featured", out.Sections[1].ID)
		assert.Len(t, out.Sections[0].Events, 3)
	})

	t.Run("home cache expires after TTL", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetHome(ctx, sampleHomeSections())

		// Fast-forward miniredis clock past TTLHome (60s)
		mr.FastForward(61 * time.Second)

		_, ok := ec.GetHome(ctx)
		assert.False(t, ok, "home cache should have expired")
	})

	t.Run("BustOnPublish clears home key", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetHome(ctx, sampleHomeSections())
		_, ok := ec.GetHome(ctx)
		require.True(t, ok, "should be cached before bust")

		ec.BustOnPublish(ctx)

		_, ok = ec.GetHome(ctx)
		assert.False(t, ok, "home key should be gone after BustOnPublish")
	})
}

// ─── Event lists ──────────────────────────────────────────────────────────────

func TestCacheTrending(t *testing.T) {
	ctx := context.Background()

	t.Run("miss returns false", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		_, ok := ec.GetTrending(ctx, 20)
		assert.False(t, ok)
	})

	t.Run("SetTrending then GetTrending round-trips correctly", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		events := sampleEvents(5)
		cursor := &utils.Cursor{ID: "last-id"}
		ec.SetTrending(ctx, 20, cache.EventPage{Events: events, NextCursor: cursor})

		page, ok := ec.GetTrending(ctx, 20)
		require.True(t, ok)
		require.Len(t, page.Events, 5)
		assert.Equal(t, "Event 1", page.Events[0].Title)
		assert.Equal(t, "last-id", page.NextCursor.ID)
	})

	t.Run("different limits produce independent keys", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetTrending(ctx, 10, cache.EventPage{Events: sampleEvents(10)})
		ec.SetTrending(ctx, 20, cache.EventPage{Events: sampleEvents(20)})

		page10, ok10 := ec.GetTrending(ctx, 10)
		page20, ok20 := ec.GetTrending(ctx, 20)

		require.True(t, ok10)
		require.True(t, ok20)
		assert.Len(t, page10.Events, 10)
		assert.Len(t, page20.Events, 20)
	})

	t.Run("expires after TTLTrending (60s)", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetTrending(ctx, 20, cache.EventPage{Events: sampleEvents(3)})
		mr.FastForward(61 * time.Second)

		_, ok := ec.GetTrending(ctx, 20)
		assert.False(t, ok)
	})

	t.Run("BustOnTicketSale clears trending and home but not featured", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetHome(ctx, sampleHomeSections())
		ec.SetTrending(ctx, 10, cache.EventPage{Events: sampleEvents(3)})
		ec.SetTrending(ctx, 20, cache.EventPage{Events: sampleEvents(5)})
		ec.SetFeatured(ctx, 10, cache.EventPage{Events: sampleEvents(2)})

		ec.BustOnTicketSale(ctx)

		_, homeCached := ec.GetHome(ctx)
		_, trend10Cached := ec.GetTrending(ctx, 10)
		_, trend20Cached := ec.GetTrending(ctx, 20)
		_, featCached := ec.GetFeatured(ctx, 10)

		assert.False(t, homeCached, "home should be busted")
		assert.False(t, trend10Cached, "trending:l10 should be busted")
		assert.False(t, trend20Cached, "trending:l20 should be busted")
		assert.True(t, featCached, "featured should NOT be busted by ticket sale")
	})
}

func TestCacheFeatured(t *testing.T) {
	ctx := context.Background()

	t.Run("SetFeatured then GetFeatured round-trips", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetFeatured(ctx, 10, cache.EventPage{Events: sampleEvents(4)})
		page, ok := ec.GetFeatured(ctx, 10)
		require.True(t, ok)
		assert.Len(t, page.Events, 4)
	})

	t.Run("expires after TTLFeatured (120s)", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetFeatured(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		mr.FastForward(121 * time.Second)
		_, ok := ec.GetFeatured(ctx, 10)
		assert.False(t, ok)
	})

	t.Run("BustOnFeaturedChange clears featured and home but not trending", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetHome(ctx, sampleHomeSections())
		ec.SetFeatured(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		ec.SetFeatured(ctx, 20, cache.EventPage{Events: sampleEvents(4)})
		ec.SetTrending(ctx, 10, cache.EventPage{Events: sampleEvents(3)})

		ec.BustOnFeaturedChange(ctx)

		_, homeCached := ec.GetHome(ctx)
		_, feat10Cached := ec.GetFeatured(ctx, 10)
		_, feat20Cached := ec.GetFeatured(ctx, 20)
		_, trendCached := ec.GetTrending(ctx, 10)

		assert.False(t, homeCached, "home should be busted")
		assert.False(t, feat10Cached, "featured:l10 should be busted")
		assert.False(t, feat20Cached, "featured:l20 should be busted")
		assert.True(t, trendCached, "trending should NOT be busted by featured change")
	})
}

func TestCacheHappeningSoon(t *testing.T) {
	ctx := context.Background()

	t.Run("round-trip", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetHappeningSoon(ctx, 10, cache.EventPage{Events: sampleEvents(3)})
		page, ok := ec.GetHappeningSoon(ctx, 10)
		require.True(t, ok)
		assert.Len(t, page.Events, 3)
	})

	t.Run("BustOnPublish clears happening_soon", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetHappeningSoon(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		ec.BustOnPublish(ctx)
		_, ok := ec.GetHappeningSoon(ctx, 10)
		assert.False(t, ok)
	})
}

func TestCacheRecentlyAdded(t *testing.T) {
	ctx := context.Background()

	t.Run("round-trip", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetRecentlyAdded(ctx, 20, cache.EventPage{Events: sampleEvents(6)})
		page, ok := ec.GetRecentlyAdded(ctx, 20)
		require.True(t, ok)
		assert.Len(t, page.Events, 6)
	})

	t.Run("expires after TTLRecentlyAdded (30s)", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetRecentlyAdded(ctx, 20, cache.EventPage{Events: sampleEvents(3)})
		mr.FastForward(31 * time.Second)
		_, ok := ec.GetRecentlyAdded(ctx, 20)
		assert.False(t, ok)
	})
}

func TestCacheThisWeekend(t *testing.T) {
	ctx := context.Background()

	t.Run("round-trip", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetThisWeekend(ctx, 10, cache.EventPage{Events: sampleEvents(4)})
		page, ok := ec.GetThisWeekend(ctx, 10)
		require.True(t, ok)
		assert.Len(t, page.Events, 4)
	})

	t.Run("expires after TTLThisWeekend (300s)", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetThisWeekend(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		mr.FastForward(301 * time.Second)
		_, ok := ec.GetThisWeekend(ctx, 10)
		assert.False(t, ok)
	})
}

func TestCacheCategory(t *testing.T) {
	ctx := context.Background()

	t.Run("round-trip with category ID", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetCategory(ctx, 3, 20, cache.EventPage{Events: sampleEvents(5)})
		page, ok := ec.GetCategory(ctx, 3, 20)
		require.True(t, ok)
		assert.Len(t, page.Events, 5)
	})

	t.Run("different category IDs have independent keys", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetCategory(ctx, 1, 20, cache.EventPage{Events: sampleEvents(3)})
		ec.SetCategory(ctx, 2, 20, cache.EventPage{Events: sampleEvents(7)})

		page1, ok1 := ec.GetCategory(ctx, 1, 20)
		page2, ok2 := ec.GetCategory(ctx, 2, 20)

		require.True(t, ok1)
		require.True(t, ok2)
		assert.Len(t, page1.Events, 3)
		assert.Len(t, page2.Events, 7)
	})

	t.Run("BustOnEventUpdate clears all events:* keys including categories", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetCategory(ctx, 1, 20, cache.EventPage{Events: sampleEvents(3)})
		ec.SetTrending(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		ec.SetFeatured(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		ec.SetHome(ctx, sampleHomeSections())

		ec.BustOnEventUpdate(ctx)

		_, catCached := ec.GetCategory(ctx, 1, 20)
		_, trendCached := ec.GetTrending(ctx, 10)
		_, featCached := ec.GetFeatured(ctx, 10)
		_, homeCached := ec.GetHome(ctx)

		assert.False(t, catCached, "category cache should be busted")
		assert.False(t, trendCached, "trending cache should be busted")
		assert.False(t, featCached, "featured cache should be busted")
		assert.False(t, homeCached, "home cache should be busted")
	})
}

// ─── Merch ────────────────────────────────────────────────────────────────────

func TestCacheMerchPreview(t *testing.T) {
	ctx := context.Background()

	t.Run("miss returns false", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		items, ok := ec.GetMerchPreview(ctx, "event-abc")
		assert.False(t, ok)
		assert.Nil(t, items)
	})

	t.Run("SetMerchPreview then GetMerchPreview round-trips", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		in := sampleMerch(4)
		ec.SetMerchPreview(ctx, "event-abc", in)

		out, ok := ec.GetMerchPreview(ctx, "event-abc")
		require.True(t, ok)
		require.Len(t, out, 4)
		assert.Equal(t, "Item 1", out[0].Name)
		assert.Equal(t, float64(100), out[0].BasePrice)
	})

	t.Run("different eventIDs have independent preview keys", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetMerchPreview(ctx, "event-1", sampleMerch(2))
		ec.SetMerchPreview(ctx, "event-2", sampleMerch(4))

		out1, ok1 := ec.GetMerchPreview(ctx, "event-1")
		out2, ok2 := ec.GetMerchPreview(ctx, "event-2")

		require.True(t, ok1)
		require.True(t, ok2)
		assert.Len(t, out1, 2)
		assert.Len(t, out2, 4)
	})

	t.Run("expires after TTLMerchPreview (300s)", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetMerchPreview(ctx, "event-abc", sampleMerch(3))
		mr.FastForward(301 * time.Second)
		_, ok := ec.GetMerchPreview(ctx, "event-abc")
		assert.False(t, ok)
	})
}

func TestCacheMerchFull(t *testing.T) {
	ctx := context.Background()

	t.Run("SetMerchFull then GetMerchFull round-trips", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		in := sampleMerch(8)
		ec.SetMerchFull(ctx, "event-xyz", in)

		out, ok := ec.GetMerchFull(ctx, "event-xyz")
		require.True(t, ok)
		assert.Len(t, out, 8)
	})

	t.Run("preview and full are independent keys for same event", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetMerchPreview(ctx, "event-abc", sampleMerch(4))
		ec.SetMerchFull(ctx, "event-abc", sampleMerch(10))

		preview, previewOK := ec.GetMerchPreview(ctx, "event-abc")
		full, fullOK := ec.GetMerchFull(ctx, "event-abc")

		require.True(t, previewOK)
		require.True(t, fullOK)
		assert.Len(t, preview, 4)
		assert.Len(t, full, 10)
	})
}

func TestCacheMerchItem(t *testing.T) {
	ctx := context.Background()

	t.Run("miss returns false", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		_, ok := ec.GetMerchItem(ctx, "merch-999")
		assert.False(t, ok)
	})

	t.Run("SetMerchItem then GetMerchItem round-trips with variants and stock summary", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		in := sampleMerchDetail("merch-abc")
		ec.SetMerchItem(ctx, "merch-abc", in)

		out, ok := ec.GetMerchItem(ctx, "merch-abc")
		require.True(t, ok)
		assert.Equal(t, "merch-abc", out.ID)
		assert.Equal(t, "Test Kit", out.Name)
		assert.Len(t, out.Variants, 3)
		// Stock summary: S=5, M=10, L=0 → total=15, available=2, status=available
		assert.EqualValues(t, 15, out.StockSummary.TotalStock)
		assert.EqualValues(t, 2, out.StockSummary.AvailableCount)
		assert.Equal(t, "available", out.StockSummary.Status)
	})

	t.Run("different merch IDs have independent item keys", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		d1 := sampleMerchDetail("merch-1")
		d2 := sampleMerchDetail("merch-2")
		d2.Name = "Different Kit"

		ec.SetMerchItem(ctx, "merch-1", d1)
		ec.SetMerchItem(ctx, "merch-2", d2)

		out1, ok1 := ec.GetMerchItem(ctx, "merch-1")
		out2, ok2 := ec.GetMerchItem(ctx, "merch-2")

		require.True(t, ok1)
		require.True(t, ok2)
		assert.Equal(t, "Test Kit", out1.Name)
		assert.Equal(t, "Different Kit", out2.Name)
	})

	t.Run("expires after TTLMerchItem (300s)", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetMerchItem(ctx, "merch-abc", sampleMerchDetail("merch-abc"))
		mr.FastForward(301 * time.Second)
		_, ok := ec.GetMerchItem(ctx, "merch-abc")
		assert.False(t, ok)
	})
}

// ─── BustMerch ────────────────────────────────────────────────────────────────

func TestCacheBustMerch(t *testing.T) {
	ctx := context.Background()

	t.Run("BustMerch with no IDs clears preview and full for that event only", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		// Seed two events
		ec.SetMerchPreview(ctx, "event-A", sampleMerch(2))
		ec.SetMerchFull(ctx, "event-A", sampleMerch(5))
		ec.SetMerchPreview(ctx, "event-B", sampleMerch(3))
		ec.SetMerchFull(ctx, "event-B", sampleMerch(6))

		// Only bust event-A
		ec.BustMerch(ctx, "event-A")

		_, aPreview := ec.GetMerchPreview(ctx, "event-A")
		_, aFull := ec.GetMerchFull(ctx, "event-A")
		_, bPreview := ec.GetMerchPreview(ctx, "event-B")
		_, bFull := ec.GetMerchFull(ctx, "event-B")

		assert.False(t, aPreview, "event-A preview should be busted")
		assert.False(t, aFull, "event-A full should be busted")
		assert.True(t, bPreview, "event-B preview should NOT be busted")
		assert.True(t, bFull, "event-B full should NOT be busted")
	})

	t.Run("BustMerch with merch IDs also clears individual item keys", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetMerchPreview(ctx, "event-A", sampleMerch(2))
		ec.SetMerchFull(ctx, "event-A", sampleMerch(5))
		ec.SetMerchItem(ctx, "merch-1", sampleMerchDetail("merch-1"))
		ec.SetMerchItem(ctx, "merch-2", sampleMerchDetail("merch-2"))

		// Bust event-A and merch-1 specifically
		ec.BustMerch(ctx, "event-A", "merch-1")

		_, aPreview := ec.GetMerchPreview(ctx, "event-A")
		_, aFull := ec.GetMerchFull(ctx, "event-A")
		_, merch1Cached := ec.GetMerchItem(ctx, "merch-1")
		_, merch2Cached := ec.GetMerchItem(ctx, "merch-2")

		assert.False(t, aPreview, "preview should be busted")
		assert.False(t, aFull, "full should be busted")
		assert.False(t, merch1Cached, "merch-1 item should be busted")
		assert.True(t, merch2Cached, "merch-2 item should NOT be busted")
	})

	t.Run("BustMerch with multiple merch IDs clears all of them", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.SetMerchItem(ctx, "merch-1", sampleMerchDetail("merch-1"))
		ec.SetMerchItem(ctx, "merch-2", sampleMerchDetail("merch-2"))
		ec.SetMerchItem(ctx, "merch-3", sampleMerchDetail("merch-3"))

		ec.BustMerch(ctx, "event-A", "merch-1", "merch-2", "merch-3")

		_, ok1 := ec.GetMerchItem(ctx, "merch-1")
		_, ok2 := ec.GetMerchItem(ctx, "merch-2")
		_, ok3 := ec.GetMerchItem(ctx, "merch-3")

		assert.False(t, ok1)
		assert.False(t, ok2)
		assert.False(t, ok3)
	})
}

// ─── BustOnPublish comprehensive ─────────────────────────────────────────────

func TestCacheBustOnPublish(t *testing.T) {
	ctx := context.Background()

	t.Run("BustOnPublish clears home, trending, happening_soon, recently_added, this_weekend", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		// Populate everything
		ec.SetHome(ctx, sampleHomeSections())
		ec.SetTrending(ctx, 10, cache.EventPage{Events: sampleEvents(3)})
		ec.SetTrending(ctx, 20, cache.EventPage{Events: sampleEvents(5)})
		ec.SetHappeningSoon(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		ec.SetHappeningSoon(ctx, 20, cache.EventPage{Events: sampleEvents(4)})
		ec.SetRecentlyAdded(ctx, 10, cache.EventPage{Events: sampleEvents(3)})
		ec.SetRecentlyAdded(ctx, 20, cache.EventPage{Events: sampleEvents(6)})
		ec.SetThisWeekend(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
		ec.SetThisWeekend(ctx, 20, cache.EventPage{Events: sampleEvents(4)})
		// Featured should NOT be busted by publish
		ec.SetFeatured(ctx, 10, cache.EventPage{Events: sampleEvents(2)})

		ec.BustOnPublish(ctx)

		_, homeCached := ec.GetHome(ctx)
		_, trend10 := ec.GetTrending(ctx, 10)
		_, trend20 := ec.GetTrending(ctx, 20)
		_, soon10 := ec.GetHappeningSoon(ctx, 10)
		_, soon20 := ec.GetHappeningSoon(ctx, 20)
		_, recent10 := ec.GetRecentlyAdded(ctx, 10)
		_, recent20 := ec.GetRecentlyAdded(ctx, 20)
		_, wknd10 := ec.GetThisWeekend(ctx, 10)
		_, wknd20 := ec.GetThisWeekend(ctx, 20)
		_, featCached := ec.GetFeatured(ctx, 10)

		assert.False(t, homeCached)
		assert.False(t, trend10)
		assert.False(t, trend20)
		assert.False(t, soon10)
		assert.False(t, soon20)
		assert.False(t, recent10)
		assert.False(t, recent20)
		assert.False(t, wknd10)
		assert.False(t, wknd20)
		assert.True(t, featCached, "featured should survive BustOnPublish")
	})
}

// ─── Redis unavailability ─────────────────────────────────────────────────────

func TestCacheRedisUnavailable(t *testing.T) {
	ctx := context.Background()

	t.Run("all Get methods return false when Redis is down", func(t *testing.T) {
		ec, mr := newTestCache(t)
		// Populate then kill Redis
		ec.SetHome(ctx, sampleHomeSections())
		ec.SetTrending(ctx, 20, cache.EventPage{Events: sampleEvents(3)})
		ec.SetMerchFull(ctx, "event-abc", sampleMerch(4))
		ec.SetMerchItem(ctx, "merch-abc", sampleMerchDetail("merch-abc"))
		mr.Close() // Redis is now down

		_, homeOK := ec.GetHome(ctx)
		_, trendOK := ec.GetTrending(ctx, 20)
		_, fullOK := ec.GetMerchFull(ctx, "event-abc")
		_, itemOK := ec.GetMerchItem(ctx, "merch-abc")

		assert.False(t, homeOK, "GetHome should return false when Redis is down")
		assert.False(t, trendOK, "GetTrending should return false when Redis is down")
		assert.False(t, fullOK, "GetMerchFull should return false when Redis is down")
		assert.False(t, itemOK, "GetMerchItem should return false when Redis is down")
	})

	t.Run("all Set methods are no-ops when Redis is down — no panic", func(t *testing.T) {
		ec, mr := newTestCache(t)
		mr.Close() // Redis is already down

		// None of these should panic
		assert.NotPanics(t, func() {
			ec.SetHome(ctx, sampleHomeSections())
			ec.SetTrending(ctx, 20, cache.EventPage{Events: sampleEvents(3)})
			ec.SetFeatured(ctx, 10, cache.EventPage{Events: sampleEvents(2)})
			ec.SetMerchPreview(ctx, "event-abc", sampleMerch(4))
			ec.SetMerchFull(ctx, "event-abc", sampleMerch(8))
			ec.SetMerchItem(ctx, "merch-abc", sampleMerchDetail("merch-abc"))
			ec.BustOnPublish(ctx)
			ec.BustOnFeaturedChange(ctx)
			ec.BustOnTicketSale(ctx)
			ec.BustOnEventUpdate(ctx)
			ec.BustMerch(ctx, "event-abc", "merch-abc")
		})
	})
}