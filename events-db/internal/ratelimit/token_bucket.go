package ratelimit

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// ==========================================
// Token Bucket Configuration
// ==========================================

type BucketConfig struct {
	Capacity   float64       // Maximum number of tokens in bucket
	RefillRate float64       // Tokens added per second
	TTL        time.Duration // How long to keep bucket in Redis
}

// Common configurations
var (
	// Login endpoints - strict (5 requests, refill 1 per minute)
	LoginBucket = BucketConfig{
		Capacity:   5,
		RefillRate: 1.0 / 60.0, // 1 token per 60 seconds
		TTL:        1 * time.Hour,
	}

	// API read endpoints - moderate (100 requests, refill 2 per second)
	APIReadBucket = BucketConfig{
		Capacity:   100,
		RefillRate: 2.0, // 2 tokens per second = 120/min
		TTL:        15 * time.Minute,
	}

	// API write endpoints - moderate (20 requests, refill 0.5 per second)
	APIWriteBucket = BucketConfig{
		Capacity:   20,
		RefillRate: 0.5, // 0.5 tokens per second = 30/min
		TTL:        15 * time.Minute,
	}

	// Admin endpoints - lenient (50 requests, refill 1 per second)
	AdminBucket = BucketConfig{
		Capacity:   50,
		RefillRate: 1.0, // 1 token per second = 60/min
		TTL:        30 * time.Minute,
	}

	// Global per-IP limit - prevent abuse (200 requests, refill 5 per second)
	GlobalIPBucket = BucketConfig{
		Capacity:   200,
		RefillRate: 5.0, // 5 tokens per second = 300/min
		TTL:        1 * time.Hour,
	}
)

// ==========================================
// Token Bucket State (stored in Redis)
// ==========================================

type BucketState struct {
	Tokens     float64 `json:"tokens"`      // Current number of tokens
	LastRefill int64   `json:"last_refill"` // Unix timestamp (milliseconds)
}

// ==========================================
// Token Bucket Limiter
// ==========================================

type TokenBucketLimiter struct {
	client *redis.Client
	config BucketConfig
}

func NewTokenBucketLimiter(client *redis.Client, config BucketConfig) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		client: client,
		config: config,
	}
}

// ==========================================
// Allow - Main rate limiting function
// ==========================================

type AllowResult struct {
	Allowed         bool    // Whether request is allowed
	RemainingTokens float64 // Tokens remaining after this request
	RetryAfter      int     // Seconds until next token available (if denied)
	ResetTime       int64   // Unix timestamp when bucket will be full
}

func (tbl *TokenBucketLimiter) Allow(ctx context.Context, key string) (*AllowResult, error) {
	return tbl.AllowN(ctx, key, 1)
}

// AllowN - Allow request that costs N tokens
func (tbl *TokenBucketLimiter) AllowN(ctx context.Context, key string, cost float64) (*AllowResult, error) {
	now := time.Now().UnixMilli()

	// Use Lua script for atomic operation
	result, err := tbl.executeTokenBucketScript(ctx, key, cost, now)
	if err != nil {
		return nil, fmt.Errorf("failed to execute rate limit: %w", err)
	}

	return result, nil
}

// ==========================================
// Redis Lua Script (Atomic Operation)
// ==========================================

// This script ensures atomicity - no race conditions
var tokenBucketScript = redis.NewScript(`
-- KEYS[1] = rate limit key
-- ARGV[1] = capacity (max tokens)
-- ARGV[2] = refill rate (tokens per second)
-- ARGV[3] = cost (tokens to consume)
-- ARGV[4] = current timestamp (milliseconds)
-- ARGV[5] = TTL (seconds)

local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local now = tonumber(ARGV[4])
local ttl = tonumber(ARGV[5])

-- Get current bucket state
local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

-- Initialize bucket if it doesn't exist
if not tokens then
    tokens = capacity
    last_refill = now
end

-- Calculate tokens to add based on time passed
local time_passed = math.max(0, now - last_refill) / 1000.0 -- Convert to seconds
local tokens_to_add = time_passed * refill_rate

-- Update tokens (capped at capacity)
tokens = math.min(capacity, tokens + tokens_to_add)

-- Check if request can be allowed
local allowed = 0
local retry_after = 0

if tokens >= cost then
    -- Allow request
    allowed = 1
    tokens = tokens - cost
else
    -- Deny request - calculate retry after
    local tokens_needed = cost - tokens
    retry_after = math.ceil(tokens_needed / refill_rate)
end

-- Save updated state
redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, ttl)

-- Calculate time until bucket is full (for reset time)
local time_to_full = (capacity - tokens) / refill_rate

-- Return: allowed, remaining tokens, retry_after, time_to_full
return {allowed, tokens, retry_after, math.ceil(time_to_full)}
`)

func (tbl *TokenBucketLimiter) executeTokenBucketScript(ctx context.Context, key string, cost float64, now int64) (*AllowResult, error) {
	keys := []string{key}
	args := []interface{}{
		tbl.config.Capacity,
		tbl.config.RefillRate,
		cost,
		now,
		int(tbl.config.TTL.Seconds()),
	}

	result, err := tokenBucketScript.Run(ctx, tbl.client, keys, args...).Result()
	if err != nil {
		return nil, err
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 4 {
		return nil, fmt.Errorf("unexpected script result")
	}

	allowed := values[0].(int64) == 1

	var tokensFloat float64
	switch v := values[1].(type) {
	case int64:
		tokensFloat = float64(v)
	case float64:
		tokensFloat = v
	default:
		return nil, fmt.Errorf("unexpected tokens type: %T", v)
	}
	retryAfter := int(values[2].(int64))
	timeToFull := int(values[3].(int64))

	return &AllowResult{
		Allowed:         allowed,
		RemainingTokens: tokensFloat,
		RetryAfter:      retryAfter,
		ResetTime:       now/1000 + int64(timeToFull),
	}, nil
}

 func parseFloat(s string) (float64, error) {
 	var f float64
 	_, err := fmt.Sscanf(s, "%f", &f)
 	return f, err
 }

// ==========================================
// Peek - Check bucket status without consuming
// ==========================================

func (tbl *TokenBucketLimiter) Peek(ctx context.Context, key string) (float64, error) {
	result, err := tbl.client.HMGet(ctx, key, "tokens", "last_refill").Result()
	if err != nil {
		return 0, err
	}

	// Bucket doesn't exist yet → full capacity
	if result[0] == nil || result[1] == nil {
		return tbl.config.Capacity, nil
	}

	// Parse stored values
	tokensStr := result[0].(string)
	lastRefillStr := result[1].(string)

	tokens, err := parseFloat(tokensStr)
	if err != nil {
		return 0, err
	}

	lastRefillMs, err := parseFloat(lastRefillStr)
	if err != nil {
		return 0, err
	}

	nowMs := float64(time.Now().UnixMilli())

	// Calculate refill
	timePassed := math.Max(0, nowMs-lastRefillMs) / 1000.0
	tokensToAdd := timePassed * tbl.config.RefillRate

	currentTokens := math.Min(
		tbl.config.Capacity,
		tokens+tokensToAdd,
	)

	return currentTokens, nil
}

// ==========================================
// Reset - Clear rate limit for a key
// ==========================================

func (tbl *TokenBucketLimiter) Reset(ctx context.Context, key string) error {
	return tbl.client.Del(ctx, key).Err()
}