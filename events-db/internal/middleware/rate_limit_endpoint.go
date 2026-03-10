package middleware

import (
	"fmt"
	"net/http"

	"events-db/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

// ==========================================
// Endpoint-Specific Rate Limiting
// ==========================================

func RateLimitEndpoint(manager *ratelimit.Manager, config ratelimit.BucketConfig, endpoint string) gin.HandlerFunc {
	limiter := manager.GetLimiter("endpoint-"+endpoint, config)

	return func(c *gin.Context) {
		// Use user ID if authenticated, otherwise IP
		identifier := c.ClientIP()
		if userID, exists := c.Get("user_id"); exists {
			identifier = userID.(string)
		}

		key := ratelimit.EndpointKey(endpoint, identifier)

		result, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			fmt.Printf("Endpoint rate limit error: %v\n", err)
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%.0f", config.Capacity))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%.0f", result.RemainingTokens))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetTime))

		if !result.Allowed {
			c.Header("Retry-After", fmt.Sprintf("%d", result.RetryAfter))
			
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       fmt.Sprintf("Rate limit exceeded for %s", endpoint),
				"message":     "Too many requests to this endpoint.",
				"retry_after": result.RetryAfter,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}