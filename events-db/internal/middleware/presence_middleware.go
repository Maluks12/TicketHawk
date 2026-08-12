package middleware

import (
	"context"
	"time"

	"events-db/internal/cache"

	"github.com/gin-gonic/gin"
)

// TrackPresence records that an authenticated user was seen.
//
// REGISTERED GLOBALLY, ONCE, ON THE ROOT ROUTER — not per group:
//
//	router.Use(middleware.TrackPresence(eventCache))
//
// It works there because of the ordering. The middleware calls c.Next() FIRST,
// letting the whole chain run, and only then reads user_id from the context.
// AuthMiddleware sets that key partway through, so by the time control returns
// here it is populated for every authenticated route regardless of which group
// the route lives in. One registration covers organiser, admin, attendee and
// anything added later — nobody has to remember to opt a new group in.
//
// Requests that never authenticated simply have no user_id and are skipped, so
// the public feed costs nothing.
func TrackPresence(ec *cache.EventCache) gin.HandlerFunc {
	if ec == nil {
		// No cache configured — hand back a pass-through rather than nil, so
		// the caller can register unconditionally.
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		c.Next()

		// CORS preflights carry no identity and would otherwise inflate the
		// count with browsers rather than people.
		if c.Request.Method == "OPTIONS" {
			return
		}

		v, ok := c.Get("user_id")
		if !ok {
			return
		}
		userID, ok := v.(string)
		if !ok || userID == "" {
			return
		}

		// Detached context on purpose. The request's context is cancelled the
		// instant the response is written, so using it here would abort a
		// meaningful share of these writes — and the ones lost would be the
		// fastest requests, which is a biased sample rather than a random one.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ec.TouchPresence(ctx, userID)
		}()
	}
}