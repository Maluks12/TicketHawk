package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// ── Guest booking access guard ────────────────────────────────────────────────
//
// Every guest endpoint authenticates with the same credential pair:
//
//	bookingNumber (path) + email (query)
//
// The old design put a 5-strike Redis lockout inside GetGuestBookingHandler and
// GetGuestTicketAssignmentsHandler only. That was wrong in both directions:
//
//   - Enumeration was never actually blocked. /payment, /refund-quote,
//     /offline, /pool-qr and POST /refund accept the identical credential with
//     no lockout at all, so an attacker simply guessed the email against one of
//     those instead.
//
//   - The lockout key was the booking number alone, so anyone holding a booking
//     number (they appear in emails, screenshots and receipts) could burn five
//     bad attempts and lock the legitimate ticket holder out of their own
//     ticket for an hour. A targeted denial of service.
//
// This middleware replaces both. It sits on the whole /guest group, so any
// route added later is covered by default rather than by remembering to.
//
// Two independent counters:
//
//	booking + email  — stops credential-stuffing a known address. The real
//	                   owner sends the CORRECT email, so their counter is never
//	                   touched by someone else's wrong guesses. This is what
//	                   removes the targeted-DoS property.
//
//	IP               — stops enumeration. Guessing many different emails
//	                   creates a fresh booking+email counter each time, so only
//	                   a per-origin counter actually catches the sweep.
//
// Fail-open on Redis errors: a guest fetching their own ticket at the door must
// not be blocked because Redis blinked. Brute-force protection degrades, the
// IP rate limiter on the group still applies, and the credential is not itself
// guessable in a single shot.
const (
	guestMaxEmailAttempts = 5
	guestMaxIPAttempts    = 30
	guestLockoutWindow    = time.Hour
)

// GuestBookingGuard returns a no-op handler when rdb is nil.
//
// This is the item-6 fix. main.go leaves the Redis client nil whenever Redis is
// unreachable AT STARTUP (rateLimitManager fails, the process logs a warning
// and continues by design). The old handler then called rdb.Get on that nil
// client, dereferenced a nil embedded pointer and panicked on every request.
// gin.Recovery() turned that into a 500 rather than a crash, so the symptom was
// a guest booking lookup that was simply broken until the API was restarted —
// most likely after a compose restart where Redis came up slower than the API.
func GuestBookingGuard(rdb *redis.Client) gin.HandlerFunc {
	if rdb == nil {
		log.Warn().Msg("guest guard: redis unavailable — guest brute-force lockout DISABLED")
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := normalizeGuestEmail(c.Query("email"))

		// Routes without the credential pair (e.g. POST /guest/bookings) are
		// not identity lookups; nothing to guard.
		if bookingNumber == "" || email == "" {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		emailKey := guestEmailKey(bookingNumber, email)
		ipKey := guestIPKey(c.ClientIP())

		if guestOverLimit(ctx, rdb, emailKey, guestMaxEmailAttempts) ||
			guestOverLimit(ctx, rdb, ipKey, guestMaxIPAttempts) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many failed attempts. Try again later",
			})
			return
		}

		c.Next()

		// Verdict is taken from the handler's own status code, which keeps the
		// guard decoupled from each handler's lookup logic.
		switch c.Writer.Status() {
		case http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden:
			guestRecordFailure(ctx, rdb, emailKey)
			guestRecordFailure(ctx, rdb, ipKey)
		default:
			if c.Writer.Status() < 300 {
				// Correct credential: clear this pair's strikes. The IP counter
				// deliberately survives, so a sweep cannot reset itself by
				// landing one valid lookup part-way through.
				if err := rdb.Del(ctx, emailKey).Err(); err != nil {
					log.Debug().Err(err).Msg("guest guard: failed to clear attempt counter")
				}
			}
		}
	}
}

func guestOverLimit(ctx context.Context, rdb *redis.Client, key string, max int) bool {
	n, err := rdb.Get(ctx, key).Int()
	if err != nil {
		// redis.Nil (no strikes yet) and genuine errors both fall through to
		// allowing the request — see the fail-open note above.
		return false
	}
	return n >= max
}

func guestRecordFailure(ctx context.Context, rdb *redis.Client, key string) {
	pipe := rdb.Pipeline()
	pipe.Incr(ctx, key)
	// ExpireNX sets the TTL only when the key has none, so the window is fixed
	// from the FIRST failure. A plain Expire would slide the window forward on
	// every attempt, letting an attacker hold a counter open indefinitely.
	pipe.ExpireNX(ctx, key, guestLockoutWindow)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Debug().Err(err).Msg("guest guard: failed to record attempt")
	}
}

func normalizeGuestEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// guestEmailKey hashes the address so raw guest emails never sit in Redis as
// plaintext keys.
func guestEmailKey(bookingNumber, normalizedEmail string) string {
	sum := sha256.Sum256([]byte(bookingNumber + "|" + normalizedEmail))
	return "guest:fail:bn:" + hex.EncodeToString(sum[:8])
}

func guestIPKey(ip string) string {
	return "guest:fail:ip:" + ip
}