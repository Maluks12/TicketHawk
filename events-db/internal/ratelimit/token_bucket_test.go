package ratelimit

// import (
// 	"context"
// 	"testing"
// 	"time"

// 	"github.com/redis/go-redis/v9"
// 	"github.com/stretchr/testify/assert"
// )

// func setupRedis(t *testing.T) *redis.Client {
// 	client := redis.NewClient(&redis.Options{
// 		Addr: "localhost:6379",
// 	})

// 	// Test connection
// 	ctx := context.Background()
// 	if err := client.Ping(ctx).Err(); err != nil {
// 		t.Skip("Redis not available")
// 	}

// 	// Clean up before tests
// 	client.FlushDB(ctx)

// 	return client
// }

// func TestTokenBucket_Basic(t *testing.T) {
// 	client := setupRedis(t)
// 	defer client.Close()

// 	config := BucketConfig{
// 		Capacity:   10,
// 		RefillRate: 1.0, // 1 token per second
// 		TTL:        1 * time.Minute,
// 	}

// 	limiter := NewTokenBucketLimiter(client, config)
// 	ctx := context.Background()

// 	t.Run("Allow requests under capacity", func(t *testing.T) {
// 		key := "test:basic"

// 		// First 10 requests should be allowed
// 		for i := 0; i < 10; i++ {
// 			result, err := limiter.Allow(ctx, key)
// 			assert.NoError(t, err)
// 			assert.True(t, result.Allowed, "Request %d should be allowed", i+1)
// 		}

// 		// 11th request should be denied
// 		result, err := limiter.Allow(ctx, key)
// 		assert.NoError(t, err)
// 		assert.False(t, result.Allowed, "11th request should be denied")
// 		assert.Greater(t, result.RetryAfter, 0)
// 	})

// 	t.Run("Bucket refills over time", func(t *testing.T) {
// 		key := "test:refill"

// 		// Exhaust bucket
// 		for i := 0; i < 10; i++ {
// 			limiter.Allow(ctx, key)
// 		}

// 		// Should be denied
// 		result, err := limiter.Allow(ctx, key)
// 		assert.NoError(t, err)
// 		assert.False(t, result.Allowed)

// 		// Wait for refill (1 token per second)
// 		time.Sleep(1100 * time.Millisecond)

// 		// Should be allowed again
// 		result, err = limiter.Allow(ctx, key)
// 		assert.NoError(t, err)
// 		assert.True(t, result.Allowed)
// 	})

// 	t.Run("Different keys are independent", func(t *testing.T) {
// 		key1 := "test:key1"
// 		key2 := "test:key2"

// 		// Exhaust key1
// 		for i := 0; i < 10; i++ {
// 			limiter.Allow(ctx, key1)
// 		}

// 		// key1 should be denied
// 		result, err := limiter.Allow(ctx, key1)
// 		assert.NoError(t, err)
// 		assert.False(t, result.Allowed)

// 		// key2 should still be allowed
// 		result, err = limiter.Allow(ctx, key2)
// 		assert.NoError(t, err)
// 		assert.True(t, result.Allowed)
// 	})
// }

// func TestTokenBucket_Burst(t *testing.T) {
// 	client := setupRedis(t)
// 	defer client.Close()

// 	config := BucketConfig{
// 		Capacity:   5,
// 		RefillRate: 0.5, // 0.5 tokens per second = 30/min
// 		TTL:        1 * time.Minute,
// 	}

// 	limiter := NewTokenBucketLimiter(client, config)
// 	ctx := context.Background()
// 	key := "test:burst"

// 	// Allow burst of 5 requests
// 	for i := 0; i < 5; i++ {
// 		result, err := limiter.Allow(ctx, key)
// 		assert.NoError(t, err)
// 		assert.True(t, result.Allowed)
// 	}

// 	// 6th request denied
// 	result, err := limiter.Allow(ctx, key)
// 	assert.NoError(t, err)
// 	assert.False(t, result.Allowed)

// 	// Wait 2 seconds = 1 token added
// 	time.Sleep(2100 * time.Millisecond)

// 	// Should allow 1 more request
// 	result, err = limiter.Allow(ctx, key)
// 	assert.NoError(t, err)
// 	assert.True(t, result.Allowed)
// }

// func TestTokenBucket_AllowN(t *testing.T) {
// 	client := setupRedis(t)
// 	defer client.Close()

// 	config := BucketConfig{
// 		Capacity:   10,
// 		RefillRate: 1.0,
// 		TTL:        1 * time.Minute,
// 	}

// 	limiter := NewTokenBucketLimiter(client, config)
// 	ctx := context.Background()
// 	key := "test:allowN"

// 	// Take 5 tokens at once
// 	result, err := limiter.AllowN(ctx, key, 5)
// 	assert.NoError(t, err)
// 	assert.True(t, result.Allowed)
// 	assert.InDelta(t, 5.0, result.RemainingTokens, 0.1)

// 	// Try to take 6 more (should fail - only 5 remaining)
// 	result, err = limiter.AllowN(ctx, key, 6)
// 	assert.NoError(t, err)
// 	assert.False(t, result.Allowed)

// 	// Take remaining 5
// 	result, err = limiter.AllowN(ctx, key, 5)
// 	assert.NoError(t, err)
// 	assert.True(t, result.Allowed)
// 	assert.InDelta(t, 0.0, result.RemainingTokens, 0.1)
// }

// func TestTokenBucket_Peek(t *testing.T) {
// 	client := setupRedis(t)
// 	defer client.Close()

// 	config := BucketConfig{
// 		Capacity:   10,
// 		RefillRate: 1.0,
// 		TTL:        1 * time.Minute,
// 	}

// 	limiter := NewTokenBucketLimiter(client, config)
// 	ctx := context.Background()
// 	key := "test:peek"

// 	// New bucket should have full capacity
// 	tokens, err := limiter.Peek(ctx, key)
// 	assert.NoError(t, err)
// 	assert.InDelta(t, 10.0, tokens, 0.1)

// 	// Consume 3 tokens
// 	limiter.AllowN(ctx, key, 3)

// 	// Peek should show 7 remaining
// 	tokens, err = limiter.Peek(ctx, key)
// 	assert.NoError(t, err)
// 	assert.InDelta(t, 7.0, tokens, 0.1)

// 	// Peek doesn't consume tokens
// 	tokens, err = limiter.Peek(ctx, key)
// 	assert.NoError(t, err)
// 	assert.InDelta(t, 7.0, tokens, 0.1)
// }

// func TestTokenBucket_Reset(t *testing.T) {
// 	client := setupRedis(t)
// 	defer client.Close()

// 	config := BucketConfig{
// 		Capacity:   5,
// 		RefillRate: 1.0,
// 		TTL:        1 * time.Minute,
// 	}

// 	limiter := NewTokenBucketLimiter(client, config)
// 	ctx := context.Background()
// 	key := "test:reset"

// 	// Exhaust bucket
// 	for i := 0; i < 5; i++ {
// 		limiter.Allow(ctx, key)
// 	}

// 	// Should be denied
// 	result, err := limiter.Allow(ctx, key)
// 	assert.NoError(t, err)
// 	assert.False(t, result.Allowed)

// 	// Reset
// 	err = limiter.Reset(ctx, key)
// 	assert.NoError(t, err)

// 	// Should be allowed again (full capacity)
// 	result, err = limiter.Allow(ctx, key)
// 	assert.NoError(t, err)
// 	assert.True(t, result.Allowed)
// }