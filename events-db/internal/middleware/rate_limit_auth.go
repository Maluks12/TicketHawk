package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"events-db/internal/ratelimit"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ============================================================================
// FIX 4 — auth endpoints must never run unprotected.
//
// Previously:
//   - if Redis was down at startup, main.go registered /login, /register,
//     /refresh and the reset endpoints with NO rate limiting at all
//   - if Redis failed mid-flight, RateLimitByIP logged a warning and let the
//     request through (fail-open)
//   - limiting was IP-only, so a distributed attack could hammer one account
//     from many IPs
//
// This file adds:
//   1. RateLimitAuthByIP  — like RateLimitByIP, but falls back to an
//      in-process token bucket when the manager is nil or Redis errors.
//      Auth endpoints therefore fail SAFE, never open.
//   2. RateLimitLoginByEmail — a second bucket keyed on the (hashed) email in
//      the login body, so one account can't be brute-forced from many IPs.
//
// The in-memory fallback is per-instance (not shared across containers), so
// under a Redis outage the effective limit is (limit × instances) — still a
// hard brake on brute force, and you currently run a single API container.
// ============================================================================

// ----------------------------------------------------------------------------
// In-memory token bucket (fallback when Redis is unavailable)
// ----------------------------------------------------------------------------

type memoryBucket struct {
	tokens   float64
	lastSeen time.Time
}

type memoryLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*memoryBucket
	cfg       ratelimit.BucketConfig
	lastPrune time.Time
}

func newMemoryLimiter(cfg ratelimit.BucketConfig) *memoryLimiter {
	return &memoryLimiter{
		buckets:   make(map[string]*memoryBucket),
		cfg:       cfg,
		lastPrune: time.Now(),
	}
}

// allow consumes one token for key. Returns whether the request may proceed
// and, if not, how many seconds until a token is available.
func (m *memoryLimiter) allow(key string) (allowed bool, retryAfter int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Opportunistic pruning so the map can't grow unbounded.
	if now.Sub(m.lastPrune) > m.cfg.TTL {
		for k, b := range m.buckets {
			if now.Sub(b.lastSeen) > m.cfg.TTL {
				delete(m.buckets, k)
			}
		}
		m.lastPrune = now
	}

	b, ok := m.buckets[key]
	if !ok {
		b = &memoryBucket{tokens: m.cfg.Capacity, lastSeen: now}
		m.buckets[key] = b
	}

	// Refill based on elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * m.cfg.RefillRate
	if b.tokens > m.cfg.Capacity {
		b.tokens = m.cfg.Capacity
	}
	b.lastSeen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	deficit := 1 - b.tokens
	retry := int(deficit/m.cfg.RefillRate) + 1
	return false, retry
}

// ----------------------------------------------------------------------------
// Shared response helper
// ----------------------------------------------------------------------------

func rejectRateLimited(c *gin.Context, category string, retryAfter int, capacity float64) {
	ratelimit.RecordDeny(category)
	c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": "Rate limit exceeded",
		"message": fmt.Sprintf(
			"Too many requests. Please retry after %d seconds.",
			retryAfter,
		),
		"retry_after": retryAfter,
		"limit":       capacity,
	})
	c.Abort()
}

// ----------------------------------------------------------------------------
// 1. IP rate limiting for auth endpoints — Redis primary, in-memory fallback
// ----------------------------------------------------------------------------

// RateLimitAuthByIP behaves like RateLimitByIP when Redis is healthy, but
// unlike it, degrades to an in-process limiter instead of failing open.
// manager may be nil (Redis unavailable at startup).
func RateLimitAuthByIP(manager *ratelimit.Manager, config ratelimit.BucketConfig, category string) gin.HandlerFunc {
	fallback := newMemoryLimiter(config)

	var limiter *ratelimit.TokenBucketLimiter
	if manager != nil {
		limiter = manager.GetLimiter(category, config)
	}

	return func(c *gin.Context) {
		ip := c.ClientIP()

		// Try Redis first.
		if limiter != nil {
			key := ratelimit.IPKey(category, ip)
			result, err := limiter.Allow(c.Request.Context(), key)
			if err == nil {
				c.Header("X-RateLimit-Limit", fmt.Sprintf("%.0f", config.Capacity))
				c.Header("X-RateLimit-Remaining", fmt.Sprintf("%.0f", result.RemainingTokens))
				c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetTime))

				if !result.Allowed {
					rejectRateLimited(c, category, int(result.RetryAfter), config.Capacity)
					return
				}
				ratelimit.RecordAllow(category)
				c.Next()
				return
			}

			// Redis errored — do NOT fail open on an auth endpoint.
			log.Warn().Err(err).Str("category", category).Str("ip", ip).
				Msg("auth rate limit: redis unavailable — using in-memory fallback")
			ratelimit.RecordError()
		}

		// In-memory fallback (manager nil, or Redis error above).
		allowed, retryAfter := fallback.allow("ip:" + ip)
		if !allowed {
			rejectRateLimited(c, category, retryAfter, config.Capacity)
			return
		}
		ratelimit.RecordAllow(category)
		c.Next()
	}
}

// ----------------------------------------------------------------------------
// 2. Per-account login limiting — keyed on the email in the request body
// ----------------------------------------------------------------------------

// RateLimitLoginByEmail throttles login attempts per target account, so an
// attacker rotating IPs still can't brute-force a single user's password.
// It peeks at the JSON body to read the email, then restores the body for the
// handler. Emails are normalized (lowercase/trim) and hashed before being
// used as a rate-limit key so raw addresses never land in Redis keys or logs.
//
// Requests without a parseable email pass through — the handler's binding
// will reject them with 400, and the IP limiter still applies.
func RateLimitLoginByEmail(manager *ratelimit.Manager, config ratelimit.BucketConfig, category string) gin.HandlerFunc {
	fallback := newMemoryLimiter(config)

	var limiter *ratelimit.TokenBucketLimiter
	if manager != nil {
		limiter = manager.GetLimiter(category, config)
	}

	return func(c *gin.Context) {
		// Read and restore the body so the handler can still bind it.
		bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1 MB cap
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var peek struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal(bodyBytes, &peek); err != nil || peek.Email == "" {
			// Malformed body / missing email → let the handler 400 it.
			c.Next()
			return
		}

		// Normalize + hash the email so the key is fixed-size and the raw
		// address never appears in Redis.
		normalized := strings.ToLower(strings.TrimSpace(peek.Email))
		sum := sha256.Sum256([]byte(normalized))
		emailKey := hex.EncodeToString(sum[:16])

		// Try Redis first.
		if limiter != nil {
			key := fmt.Sprintf("ratelimit:login-email:%s", emailKey)
			result, rErr := limiter.Allow(c.Request.Context(), key)
			if rErr == nil {
				if !result.Allowed {
					rejectRateLimited(c, category, int(result.RetryAfter), config.Capacity)
					return
				}
				ratelimit.RecordAllow(category)
				c.Next()
				return
			}
			log.Warn().Err(rErr).Str("category", category).
				Msg("login email rate limit: redis unavailable — using in-memory fallback")
			ratelimit.RecordError()
		}

		// In-memory fallback.
		allowed, retryAfter := fallback.allow("email:" + emailKey)
		if !allowed {
			rejectRateLimited(c, category, retryAfter, config.Capacity)
			return
		}
		ratelimit.RecordAllow(category)
		c.Next()
	}
}