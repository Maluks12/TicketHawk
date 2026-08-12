package handlers

import (
	"net/http"
	"time"

	"events-db/internal/cache"

	"github.com/gin-gonic/gin"
)

// GET /admin/stats/presence
//
// Deliberately its OWN endpoint rather than another field on /admin/stats/gmv.
//
// The console polls this every 60 seconds, and the GMV handler runs four
// aggregate queries across bookings, merch, events and users. Polling that once
// a minute per open admin tab would put a standing load on the database to
// refresh one number. This touches Redis only — three ZCOUNTs against a set
// sized by today's active users — so it costs effectively nothing to call
// often.
//
// Gated on the weakest staff capability at the route: it is a count with no
// personal data in it.
func GetPresenceHandler(ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ec == nil {
			// Presence needs Redis. Reporting `available: false` rather than
			// zeroes lets the console say "not available" instead of drawing a
			// confident 0, which would read as "nobody is using the product".
			c.JSON(http.StatusOK, gin.H{"available": false})
			return
		}

		ctx := c.Request.Context()

		// Three windows from one sorted set. Five minutes is the headline:
		// long enough that someone reading a page still counts, short enough
		// that the number moves and means "now".
		c.JSON(http.StatusOK, gin.H{
			"available":   true,
			"active_now":  ec.CountActiveSince(ctx, 5*time.Minute),
			"active_1h":   ec.CountActiveSince(ctx, time.Hour),
			"active_24h":  ec.CountActiveSince(ctx, 24*time.Hour),
			"window_secs": 300,
			"as_of":       time.Now(),
		})
	}
}