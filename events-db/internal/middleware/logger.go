package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		requestID, _ := c.Get("request_id")
		log.Info().
		Str("request_id", requestID.(string)).
		Str("method",     c.Request.Method).
		Str("path",       c.Request.URL.Path).
		Int("status",     c.Writer.Status()).
		Dur("latency",    time.Since(start)).
		Str("ip",         c.ClientIP()).
		Msg("request")
	}
}