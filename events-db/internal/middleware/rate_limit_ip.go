package middleware

import (
	"fmt"
	"net/http"

	"events-db/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

func RateLimitByIP(manager *ratelimit.Manager, config ratelimit.BucketConfig, category string) gin.HandlerFunc {
	limiter := manager.GetLimiter(category, config)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := ratelimit.IPKey(category, ip)

		result, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			// Log error and record metric
			fmt.Printf("Rate limit error: %v\n", err)
			ratelimit.RecordError()
			c.Next()
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%.0f", config.Capacity))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%.0f", result.RemainingTokens))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetTime))

		if !result.Allowed {
			// Record denied request
			ratelimit.RecordDeny(category)
			
			c.Header("Retry-After", fmt.Sprintf("%d", result.RetryAfter))
			
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
				"message": fmt.Sprintf(
					"Too many requests. Please retry after %d seconds.",
					result.RetryAfter,
				),
				"retry_after": result.RetryAfter,
				"limit":       config.Capacity,
			})
			c.Abort()
			return
		}

		// Record allowed request
		ratelimit.RecordAllow(category)

		c.Next()
	}
}

func GlobalIPRateLimit(manager *ratelimit.Manager) gin.HandlerFunc {
	limiter := manager.GetLimiter("global-ip", ratelimit.GlobalIPBucket)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := ratelimit.GlobalIPKey(ip)

		result, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			fmt.Printf("Global rate limit error: %v\n", err)
			ratelimit.RecordError()
			c.Next()
			return
		}

		c.Header("X-Global-RateLimit-Remaining", fmt.Sprintf("%.0f", result.RemainingTokens))

		if !result.Allowed {
			ratelimit.RecordDeny("global-ip")
			
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Global rate limit exceeded",
				"message":     "You have made too many requests across all endpoints.",
				"retry_after": result.RetryAfter,
			})
			c.Abort()
			return
		}

		ratelimit.RecordAllow("global-ip")

		c.Next()
	}
}