package ratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv" // FIX 9: replaces fmt for parseFloat
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
	// 10 attempts, refills 1 token per minute. Prevents brute force without
	// locking legitimate users out for hours.
	LoginBucket = BucketConfig{
		Capacity:   10,
		RefillRate: 1.0 / 60.0,
		TTL:        2 * time.Hour,
	}

	APIReadBucket = BucketConfig{
		Capacity:   100,
		RefillRate: 2.0,
		TTL:        15 * time.Minute,
	}

	APIWriteBucket = BucketConfig{
		Capacity:   20,
		RefillRate: 0.5,
		TTL:        15 * time.Minute,
	}

	AdminBucket = BucketConfig{
		Capacity:   50,
		RefillRate: 1.0,
		TTL:        30 * time.Minute,
	}

	GlobalIPBucket = BucketConfig{
		Capacity:   200,
		RefillRate: 5.0,
		TTL:        1 * time.Hour,
	}
)

// ==========================================
// Token Bucket State (stored in Redis)
// ==========================================

type BucketState struct {
	Tokens     float64 `json:"tokens"`
	LastRefill int64   `json:"last_refill"`
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
	Allowed         bool
	RemainingTokens float64
	RetryAfter      int
	ResetTime       int64
}

func (tbl *TokenBucketLimiter) Allow(ctx context.Context, key string) (*AllowResult, error) {
	return tbl.AllowN(ctx, key, 1)
}

func (tbl *TokenBucketLimiter) AllowN(ctx context.Context, key string, cost float64) (*AllowResult, error) {
	now := time.Now().UnixMilli()
	result, err := tbl.executeTokenBucketScript(ctx, key, cost, now)
	if err != nil {
		return nil, fmt.Errorf("failed to execute rate limit: %w", err)
	}
	return result, nil
}

// ==========================================
// Redis Lua Script (Atomic Operation)
// ==========================================

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local now = tonumber(ARGV[4])
local ttl = tonumber(ARGV[5])

local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if not tokens then
    tokens = capacity
    last_refill = now
end

local time_passed = math.max(0, now - last_refill) / 1000.0
local tokens_to_add = time_passed * refill_rate
tokens = math.min(capacity, tokens + tokens_to_add)

local allowed = 0
local retry_after = 0

if tokens >= cost then
    allowed = 1
    tokens = tokens - cost
else
    local tokens_needed = cost - tokens
    retry_after = math.ceil(tokens_needed / refill_rate)
end

redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, ttl)

local time_to_full = (capacity - tokens) / refill_rate
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

// FIX 9: parseFloat previously used fmt.Sscanf which is unidiomatic and slower.
// strconv.ParseFloat is the standard Go approach for this conversion.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// ==========================================
// Peek - Check bucket status without consuming
// ==========================================

func (tbl *TokenBucketLimiter) Peek(ctx context.Context, key string) (float64, error) {
	result, err := tbl.client.HMGet(ctx, key, "tokens", "last_refill").Result()
	if err != nil {
		return 0, err
	}

	if result[0] == nil || result[1] == nil {
		return tbl.config.Capacity, nil
	}

	tokens, err := parseFloat(result[0].(string))
	if err != nil {
		return 0, err
	}

	lastRefillMs, err := parseFloat(result[1].(string))
	if err != nil {
		return 0, err
	}

	nowMs := float64(time.Now().UnixMilli())
	timePassed := math.Max(0, nowMs-lastRefillMs) / 1000.0
	tokensToAdd := timePassed * tbl.config.RefillRate

	return math.Min(tbl.config.Capacity, tokens+tokensToAdd), nil
}

// ==========================================
// Reset - Clear rate limit for a key
// ==========================================

func (tbl *TokenBucketLimiter) Reset(ctx context.Context, key string) error {
	return tbl.client.Del(ctx, key).Err()
}