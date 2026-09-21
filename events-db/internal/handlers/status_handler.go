package handlers

import (
	"context"
	"hash/fnv"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"events-db/internal/utils"
)

// ─────────────────────────────────────────────────────────────────────────────
// Service status — the feed behind the maintenance / outage banner that every
// TicketHawk client shows at the top of every screen.
//
// ── What this endpoint is NOT ────────────────────────────────────────────────
//
// It is not how a client discovers the API is DOWN. A dead server cannot
// answer a question about its own health, so "the server is down" is detected
// client-side: a request that times out, refuses, or comes back 5xx IS the
// signal. See store/statusStore.ts (web) and lib/serviceStatus.ts (mobile).
//
// This endpoint answers the two things a client cannot work out on its own:
//
//   • maintenance — a window the operator declared ON PURPOSE. Without this
//     the apps would tell people "something went wrong" during a planned
//     deploy, which is both wrong and alarming.
//   • degraded    — the API is up and answering, but a dependency it needs is
//     not. Postgres down means every real request 500s while the process
//     itself is perfectly healthy, so from the outside it looks fine.
//
// ── Why it always returns 200 ────────────────────────────────────────────────
//
// A non-200 here would be indistinguishable from the tunnel, the edge or the
// container being broken — exactly the condition the clients already detect
// themselves. So the HTTP status says "I answered" and the BODY says how the
// platform is doing. /health keeps its 503 behaviour untouched: infra tooling
// (the compose healthcheck, wget probes) reads that one, and making it flap on
// a declared maintenance window would have Docker restart the container.
//
// ── Turning maintenance on and off ───────────────────────────────────────────
//
// One Redis key. Setting it turns the banner on for every client within a few
// seconds; deleting it turns the banner off. On the EC2 box:
//
//	docker compose -f docker-compose.prod.yml exec redis \
//	  redis-cli SET th:maintenance "Upgrading the payment system. Booking is paused until 02:00." EX 3600
//
//	docker compose -f docker-compose.prod.yml exec redis redis-cli DEL th:maintenance
//
// The EX argument is deliberate: the key's own TTL is the end of the window, so
// a forgotten banner removes itself instead of sitting on the site all weekend.
// The remaining TTL is also what populates ends_at below, which is how clients
// can render "until 02:00" without the operator typing a timestamp twice.
//
// MAINTENANCE_MESSAGE in .env.production is the fallback path, for the case
// where Redis itself is the thing being worked on. The Redis key wins when both
// are set; the env var needs a container restart to take effect, which is why
// it is the fallback and not the primary.
//
// ── Cost ─────────────────────────────────────────────────────────────────────
//
// Every client polls this, so the handler must stay cheap under the kind of
// load a real outage produces (every open app retrying at once). The answer is
// computed at most once every statusCacheTTL and served from memory; the
// refresh happens on a background goroutine, so no request ever waits on Redis
// or Postgres. A caller arriving during a refresh gets the previous answer,
// which is at most a few seconds stale — fine for a banner, and far better
// than a queue of goroutines blocked on a dead database.
// ─────────────────────────────────────────────────────────────────────────────

// MaintenanceKey is the Redis key an operator sets to declare a maintenance
// window. Value = the message shown to users. TTL = the end of the window.
const MaintenanceKey = "th:maintenance"

const (
	// Values of the "status" field. Clients switch on these strings, so they
	// are part of the API contract — add to them rather than renaming.
	StatusOK          = "ok"
	StatusDegraded    = "degraded"
	StatusMaintenance = "maintenance"

	// How long a computed answer is served before a refresh is kicked off.
	// Short enough that flipping the Redis key feels immediate, long enough
	// that a stampede of clients costs one Redis round trip, not thousands.
	statusCacheTTL = 5 * time.Second

	// Ceiling on each dependency probe. A hung dependency must never hold the
	// refresh goroutine open longer than the cache window itself.
	statusProbeTimeout = 2 * time.Second
)

// serviceStatusResponse is the wire format. Every field is safe to show a
// member of the public: no versions, hostnames, error strings or dependency
// names — for the same reason the root handler withholds them.
type serviceStatusResponse struct {
	Status  string `json:"status"`            // ok | degraded | maintenance
	Message string `json:"message,omitempty"` // operator's words, maintenance only
	EndsAt  string `json:"ends_at,omitempty"` // RFC3339 in SAST, when known

	// Revision changes only when the MEANING changes. Clients key a dismissed
	// banner on it, so a user who dismisses today's maintenance notice does not
	// get it back on every poll — but does get the next, different one.
	Revision string `json:"revision"`

	// Lets a client render "until 02:00" against the server's clock rather than
	// a device clock that may be wrong by hours.
	ServerTime string `json:"server_time"`
}

// statusSnapshot is the cached, computed view of the platform.
type statusSnapshot struct {
	status  string
	message string
	endsAt  time.Time // zero when the window has no declared end
}

// statusState holds the snapshot plus the machinery that keeps it fresh.
// Lock-free on purpose: this is read on every poll during an outage.
type statusState struct {
	snap       atomic.Pointer[statusSnapshot]
	computedAt atomic.Int64 // UnixNano of the last successful compute
	refreshing atomic.Bool  // one refresh in flight at a time
}

// current returns the cached snapshot, scheduling a refresh if it has aged out.
// The caller is never blocked — a stale-by-seconds banner beats a stalled one.
func (s *statusState) current(pool *pgxpool.Pool, rdb *redis.Client, envMsg string) statusSnapshot {
	if time.Since(time.Unix(0, s.computedAt.Load())) > statusCacheTTL {
		s.refreshAsync(pool, rdb, envMsg)
	}
	return *s.snap.Load()
}

func (s *statusState) refreshAsync(pool *pgxpool.Pool, rdb *redis.Client, envMsg string) {
	if !s.refreshing.CompareAndSwap(false, true) {
		return // another goroutine is already on it
	}
	go func() {
		defer s.refreshing.Store(false)
		// context.Background(), NOT the request context: this outlives the
		// request that triggered it, and a cancelled context here would leave
		// the cache permanently stale.
		snap := computeStatus(context.Background(), pool, rdb, envMsg)
		s.snap.Store(&snap)
		s.computedAt.Store(time.Now().UnixNano())
	}()
}

// computeStatus asks the two questions that matter, in priority order.
func computeStatus(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, envMsg string) statusSnapshot {
	// 1. A declared maintenance window outranks everything else. During a
	//    deploy the database connection may well be fine — that is not the
	//    story the user should be told.
	if rdb != nil {
		rctx, cancel := context.WithTimeout(ctx, statusProbeTimeout)
		defer cancel()

		if msg, err := rdb.Get(rctx, MaintenanceKey).Result(); err == nil {
			if trimmed := strings.TrimSpace(msg); trimmed != "" {
				snap := statusSnapshot{status: StatusMaintenance, message: trimmed}
				// TTL doubles as the end of the window. Negative values mean
				// "no expiry set" (-1) or "key gone" (-2) — neither is an end.
				if ttl, terr := rdb.TTL(rctx, MaintenanceKey).Result(); terr == nil && ttl > 0 {
					snap.endsAt = time.Now().Add(ttl)
				}
				return snap
			}
		}
	}

	// Fallback for when Redis is unreachable or is itself what is being worked
	// on. Read once at startup (see ServiceStatusHandler) because process
	// environment cannot change without a restart anyway — which is exactly why
	// this is the fallback and the Redis key is the primary switch.
	if envMsg != "" {
		return statusSnapshot{status: StatusMaintenance, message: envMsg}
	}

	// 2. Postgres. Nothing on the platform works without it, and its absence is
	//    invisible from outside: the API keeps accepting connections and keeps
	//    returning 500s. Redis is NOT probed here — the app degrades gracefully
	//    without it (rate limiting falls back in-process, caches miss), so a
	//    Redis outage is not something to put in front of a ticket buyer.
	pctx, cancel := context.WithTimeout(ctx, statusProbeTimeout)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		return statusSnapshot{status: StatusDegraded}
	}

	return statusSnapshot{status: StatusOK}
}

// revisionOf fingerprints the MEANING of a snapshot: status plus message.
// endsAt is deliberately excluded — it counts down on every refresh, and
// including it would make a dismissed banner reappear seconds later.
func revisionOf(s statusSnapshot) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s.status))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(s.message))
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}

// ServiceStatusHandler answers GET /v1/status.
//
// Registered outside the /v1 group in main.go so it sits next to /health, but
// addressed under /v1 because that is the base URL every client already holds.
// Both dependencies may be nil-ish: rdb is nil whenever the rate limiter failed
// to start, and the handler must still answer.
func ServiceStatusHandler(pool *pgxpool.Pool, rdb *redis.Client) gin.HandlerFunc {
	state := &statusState{}
	state.snap.Store(&statusSnapshot{status: StatusOK})

	envMsg := strings.TrimSpace(os.Getenv("MAINTENANCE_MESSAGE"))

	// Warm the cache before the first client asks, so a restart during a
	// maintenance window does not serve one wrong "ok" to whoever is first.
	state.refreshAsync(pool, rdb, envMsg)

	return func(c *gin.Context) {
		snap := state.current(pool, rdb, envMsg)

		// Cloudflare sits in front of this and clients poll it every few
		// seconds. A cached copy of "ok" served during an outage would defeat
		// the entire feature, so nothing anywhere may store this response.
		c.Header("Cache-Control", "no-store")

		resp := serviceStatusResponse{
			Status:     snap.status,
			Message:    snap.message,
			Revision:   revisionOf(snap),
			ServerTime: time.Now().In(utils.SALocation()).Format(time.RFC3339),
		}
		if !snap.endsAt.IsZero() {
			resp.EndsAt = snap.endsAt.In(utils.SALocation()).Format(time.RFC3339)
		}

		c.JSON(http.StatusOK, resp)
	}
}