package handlers

import (
	"net/http"
	"time"

	"events-db/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

// bucketForCategory returns the BucketConfig that matches how each category
// is configured in main.go. Keeping them here avoids duplicating magic numbers
// across the inline admin endpoints.
func bucketForIPCategory(category string) ratelimit.BucketConfig {
	switch category {
	case "login":
		// 10 attempts, refill 1 token/minute
		return ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 2 * time.Hour}
	case "register":
		// 5 attempts, refill 1 token/10 minutes
		return ratelimit.BucketConfig{Capacity: 5, RefillRate: 1.0 / 600.0, TTL: 2 * time.Hour}
	case "password-reset":
		return ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 300.0, TTL: 1 * time.Hour}
	default:
		return ratelimit.GlobalIPBucket
	}
}

func bucketForUserCategory(category string) ratelimit.BucketConfig {
	switch category {
	case "admin":
		return ratelimit.BucketConfig{Capacity: 200, RefillRate: 10.0, TTL: 30 * time.Minute}
	default:
		return ratelimit.APIReadBucket
	}
}

// RateLimitIPStatusHandler returns the current token count for a given IP + category.
// GET /admin/ratelimit/ip/:ip?category=login
func RateLimitIPStatusHandler(mgr *ratelimit.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.Param("ip")
		category := c.DefaultQuery("category", "global")
		key := ratelimit.IPKey(category, ip)
		bcfg := bucketForIPCategory(category)

		limiter := mgr.GetLimiter(category, bcfg)
		tokens, err := limiter.Peek(c.Request.Context(), key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check rate limit"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"ip":       ip,
			"category": category,
			"tokens":   tokens,
			"capacity": bcfg.Capacity,
			"status":   tokens > 0,
		})
	}
}

// RateLimitIPResetHandler refills the bucket for a given IP + category to full.
// POST /admin/ratelimit/ip/:ip/reset?category=login
func RateLimitIPResetHandler(mgr *ratelimit.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.Param("ip")
		category := c.DefaultQuery("category", "global")
		key := ratelimit.IPKey(category, ip)
		bcfg := bucketForIPCategory(category)

		limiter := mgr.GetLimiter(category, bcfg)
		if err := limiter.Reset(c.Request.Context(), key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset rate limit"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":  "rate limit reset successfully",
			"ip":       ip,
			"category": category,
		})
	}
}

// RateLimitUserStatusHandler returns the current token count for a given user + category.
// GET /admin/ratelimit/user/:userId?category=events-read
func RateLimitUserStatusHandler(mgr *ratelimit.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("userId")
		category := c.DefaultQuery("category", "events-read")
		key := ratelimit.UserKey(category, userID)
		bcfg := bucketForUserCategory(category)

		limiter := mgr.GetLimiter(category, bcfg)
		tokens, err := limiter.Peek(c.Request.Context(), key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check rate limit"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"user_id":  userID,
			"category": category,
			"tokens":   tokens,
			"capacity": bcfg.Capacity,
			"status":   tokens > 0,
		})
	}
}

// RateLimitUserResetHandler refills the bucket for a given user + category to full.
// POST /admin/ratelimit/user/:userId/reset?category=events-read
func RateLimitUserResetHandler(mgr *ratelimit.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("userId")
		category := c.DefaultQuery("category", "events-read")
		key := ratelimit.UserKey(category, userID)
		bcfg := bucketForUserCategory(category)

		limiter := mgr.GetLimiter(category, bcfg)
		if err := limiter.Reset(c.Request.Context(), key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset rate limit"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":  "rate limit reset successfully",
			"user_id":  userID,
			"category": category,
		})
	}
}

// RateLimitMetricsHandler returns aggregate allow/deny counters.
// GET /admin/ratelimit/metrics
func RateLimitMetricsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		metrics := ratelimit.GetMetrics()
		c.JSON(http.StatusOK, gin.H{
			"total_requests":   metrics.TotalRequests,
			"allowed_requests": metrics.AllowedRequests,
			"denied_requests":  metrics.DeniedRequests,
			"errors":           metrics.Errors,
			"allow_rate":       metrics.AllowRate(),
			"categories":       metrics.CategoryMetrics,
		})
	}
}