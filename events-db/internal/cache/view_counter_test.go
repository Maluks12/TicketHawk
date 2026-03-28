package cache_test

// =============================================================================
// VIEW COUNTER CACHE TESTS
// =============================================================================
//
// Tests covered:
//   IncrementView:
//     - First increment creates the key with count 1
//     - Multiple increments accumulate correctly
//     - Different event IDs have independent counters
//     - No-op (no panic) when Redis is down
//
//   FlushViews:
//     - Returns nil when no view keys exist
//     - Returns correct counts for all pending events
//     - Deletes keys after flushing (atomic GetDel)
//     - Does not affect non-view keys
//     - Multiple events flushed in one call
//     - Returns nil when Redis is down
//
// Run:
//   go test ./internal/cache/... -v -run TestViewCounter
// =============================================================================

import (
	"context"
	"events-db/internal/cache"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewCounterIncrement(t *testing.T) {
	ctx := context.Background()

	t.Run("first increment creates key with count 1", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.IncrementView(ctx, "event-001")

		counts := ec.FlushViews(ctx)
		require.NotNil(t, counts)
		assert.Equal(t, int64(1), counts["event-001"])
	})

	t.Run("multiple increments accumulate correctly", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.IncrementView(ctx, "event-001")
		ec.IncrementView(ctx, "event-001")
		ec.IncrementView(ctx, "event-001")

		counts := ec.FlushViews(ctx)
		require.NotNil(t, counts)
		assert.Equal(t, int64(3), counts["event-001"])
	})

	t.Run("different event IDs have independent counters", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.IncrementView(ctx, "event-A")
		ec.IncrementView(ctx, "event-A")
		ec.IncrementView(ctx, "event-B")

		counts := ec.FlushViews(ctx)
		require.NotNil(t, counts)
		assert.Equal(t, int64(2), counts["event-A"])
		assert.Equal(t, int64(1), counts["event-B"])
	})

	t.Run("no panic when Redis is down", func(t *testing.T) {
		ec, mr := newTestCache(t)
		mr.Close() // kill Redis before calling

		assert.NotPanics(t, func() {
			ec.IncrementView(ctx, "event-001")
		})
	})
}

func TestViewCounterFlush(t *testing.T) {
	ctx := context.Background()

	t.Run("returns nil when no view keys exist", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		counts := ec.FlushViews(ctx)
		assert.Nil(t, counts)
	})

	t.Run("keys are deleted after flush — second flush returns nil", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.IncrementView(ctx, "event-001")
		ec.IncrementView(ctx, "event-001")

		first := ec.FlushViews(ctx)
		require.NotNil(t, first)
		assert.Equal(t, int64(2), first["event-001"])

		// Keys should be gone now
		second := ec.FlushViews(ctx)
		assert.Nil(t, second, "flush should delete keys — second call should return nil")
	})

	t.Run("flushes multiple events in one call", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		ec.IncrementView(ctx, "event-A")
		ec.IncrementView(ctx, "event-A")
		ec.IncrementView(ctx, "event-B")
		ec.IncrementView(ctx, "event-B")
		ec.IncrementView(ctx, "event-B")
		ec.IncrementView(ctx, "event-C")

		counts := ec.FlushViews(ctx)
		require.NotNil(t, counts)
		require.Len(t, counts, 3)
		assert.Equal(t, int64(2), counts["event-A"])
		assert.Equal(t, int64(3), counts["event-B"])
		assert.Equal(t, int64(1), counts["event-C"])
	})

	t.Run("does not affect non-view keys", func(t *testing.T) {
		ec, mr := newTestCache(t)
		defer mr.Close()

		// Seed a trending cache entry alongside view counters
		ec.SetTrending(ctx, 10, cache.EventPage{Events: sampleEvents(3)})
		ec.IncrementView(ctx, "event-001")

		counts := ec.FlushViews(ctx)
		require.NotNil(t, counts)
		assert.Len(t, counts, 1, "only view keys should be flushed")

		// Trending cache should still be intact
		page, ok := ec.GetTrending(ctx, 10)
		require.True(t, ok, "trending cache should not be affected by FlushViews")
		assert.Len(t, page.Events, 3)
	})

	t.Run("returns nil when Redis is down", func(t *testing.T) {
		ec, mr := newTestCache(t)
		mr.Close()

		assert.NotPanics(t, func() {
			counts := ec.FlushViews(ctx)
			assert.Nil(t, counts)
		})
	})
}