package middleware

import (
	//"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitLogger logs rate limit events
func RateLimitLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// Log if rate limited
		if c.Writer.Status() == 429 {
			log.Printf(
				"RATE_LIMITED | IP: %s | Path: %s | User: %s | Duration: %v",
				c.ClientIP(),
				c.Request.URL.Path,
				c.GetString("user_id"),
				time.Since(start),
			)
		}
	}
}