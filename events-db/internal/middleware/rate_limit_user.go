package middleware

import (
	"fmt"
	"net/http"

	"events-db/internal/ratelimit"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func RateLimitByUser(manager *ratelimit.Manager, config ratelimit.BucketConfig, category string) gin.HandlerFunc {
	limiter := manager.GetLimiter(category, config)

	return func(c *gin.Context) {
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		userID := userIDInterface.(string)
		key := ratelimit.UserKey(category, userID)

		result, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			log.Warn().Err(err).Str("category", category).Str("user_id", userID).Msg("user rate limit check failed — allowing request")
			ratelimit.RecordError()
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%.0f", config.Capacity))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%.0f", result.RemainingTokens))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetTime))

		if !result.Allowed {
			ratelimit.RecordDeny(category)

			c.Header("Retry-After", fmt.Sprintf("%d", result.RetryAfter))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
				"message": fmt.Sprintf(
					"You have exceeded your rate limit. Please retry after %d seconds.",
					result.RetryAfter,
				),
				"retry_after": result.RetryAfter,
				"limit":       config.Capacity,
				"remaining":   result.RemainingTokens,
			})
			c.Abort()
			return
		}

		ratelimit.RecordAllow(category)

		c.Next()
	}
}

func CompositeRateLimit(
	manager *ratelimit.Manager,
	ipConfig ratelimit.BucketConfig,
	userConfig ratelimit.BucketConfig,
	category string,
) gin.HandlerFunc {
	ipLimiter := manager.GetLimiter(category+"-ip", ipConfig)
	userLimiter := manager.GetLimiter(category+"-user", userConfig)

	return func(c *gin.Context) {
		ip := c.ClientIP()

		ipKey := ratelimit.IPKey(category, ip)
		ipResult, err := ipLimiter.Allow(c.Request.Context(), ipKey)

		if err != nil {
			log.Warn().Err(err).Str("category", category).Str("ip", ip).Msg("composite IP rate limit check failed — skipping")
			ratelimit.RecordError()
		} else if !ipResult.Allowed {
			ratelimit.RecordDeny(category + "-ip")

			c.Header("Retry-After", fmt.Sprintf("%d", ipResult.RetryAfter))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "IP rate limit exceeded",
				"message":     "Too many requests from your IP address.",
				"retry_after": ipResult.RetryAfter,
			})
			c.Abort()
			return
		} else {
			ratelimit.RecordAllow(category + "-ip")
		}

		userIDInterface, exists := c.Get("user_id")
		if exists {
			userID := userIDInterface.(string)
			userKey := ratelimit.UserKey(category, userID)
			userResult, err := userLimiter.Allow(c.Request.Context(), userKey)

			if err != nil {
				log.Warn().Err(err).Str("category", category).Str("user_id", userID).Msg("composite user rate limit check failed — skipping")
				ratelimit.RecordError()
			} else {
				c.Header("X-RateLimit-Limit", fmt.Sprintf("%.0f", userConfig.Capacity))
				c.Header("X-RateLimit-Remaining", fmt.Sprintf("%.0f", userResult.RemainingTokens))

				if !userResult.Allowed {
					ratelimit.RecordDeny(category + "-user")

					c.Header("Retry-After", fmt.Sprintf("%d", userResult.RetryAfter))
					c.JSON(http.StatusTooManyRequests, gin.H{
						"error":       "User rate limit exceeded",
						"message":     "You have exceeded your personal rate limit.",
						"retry_after": userResult.RetryAfter,
						"limit":       userConfig.Capacity,
					})
					c.Abort()
					return
				}

				ratelimit.RecordAllow(category + "-user")
			}
		}

		c.Next()
	}
}