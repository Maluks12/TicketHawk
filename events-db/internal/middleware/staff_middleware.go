package middleware

import (
	"context"
	"net/http"
	"time"

	"events-db/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Staff capability enforcement
//
// Sits AFTER AuthMiddleware, which has already established who the caller is.
// This layer answers the separate question of what they may do in the console.
//
// Two properties are deliberate:
//
//  1. The staff role is read from the DATABASE on every request, not from the
//     JWT. Roles change during incidents — revoking a compromised account has
//     to take effect now, not when their token expires. The cost is one
//     indexed primary-key lookup per admin request; the console's traffic is
//     a handful of humans, so it is not a meaningful cost.
//
//  2. Denials are AUDITED, not just refused. A staff member repeatedly probing
//     endpoints they can't reach is the strongest early signal of a
//     compromised account, and logging only successes hides precisely that.
// ─────────────────────────────────────────────────────────────────────────────

const (
	ctxStaffRole = "staff_role"
	ctxStaffCaps = "staff_caps"
	// ctxStaffLegacy marks a platform admin who predates the staff model and
	// holds no staff_role. Treated as admin for CONSOLE capabilities, and never
	// for identity documents.
	ctxStaffLegacy = "staff_legacy_admin"
)

// RequireStaff loads the caller's staff role and rejects non-staff.
//
// Register on the admin route group ahead of any capability check.
func RequireStaff(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var roleStr *string
		var email string
		err := pool.QueryRow(ctx,
			`SELECT staff_role::text, email FROM users WHERE id = $1`,
			userID,
		).Scan(&roleStr, &email)

		if err == pgx.ErrNoRows {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if err != nil {
			log.Error().Err(err).Msg("staff middleware: role lookup failed")
			// Fail CLOSED. Unlike the organiser's legal gate — where a failed
			// check costs a prompt — a failed check here would hand out console
			// access on a database hiccup.
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}

		if roleStr == nil || *roleStr == "" {
			// LEGACY PLATFORM ADMIN — allowed through with the admin
			// capability set.
			//
			// This branch exists because refusing here locked the console's own
			// key inside. 000068 left staff_role NULL for every existing
			// account, and /admin/me/capabilities sits behind this middleware,
			// so a platform admin got a 403, the console's capability store
			// failed closed on an empty list, and Analytics, Approvals, Staff
			// Requests and People all rendered "Access denied" — including for
			// whoever needed to log in and grant the first staff_role.
			//
			// It escalates nothing. AdminOnlyMiddleware has already established
			// users.role='admin', and such an account can call every legacy
			// admin endpoint today with no restriction at all. Handing it the
			// admin capability set narrows what it can do; it does not widen
			// it. Master admin is a different column and is NOT implied here,
			// so this cannot hand anyone an identity document.
			//
			// 000074 backfills these accounts, so in practice this is a safety
			// net for rows created between deploy and migration rather than the
			// normal path.
			legacy := models.StaffAdmin
			c.Set(ctxStaffRole, legacy)
			c.Set(ctxStaffCaps, models.Capabilities(&legacy))
			c.Set(ctxStaffLegacy, true)
			c.Set("actor_email", email)
			c.Next()
			return
		}

		role, pErr := models.ParseStaffRole(*roleStr)
		if pErr != nil {
			// An unrecognised value means the enum grew and this binary is
			// older than the database. Denying is the only safe reading.
			log.Error().Str("role", *roleStr).Msg("staff middleware: unknown staff role")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "staff access required"})
			return
		}

		c.Set(ctxStaffRole, role)
		c.Set(ctxStaffCaps, models.Capabilities(&role))
		c.Set("actor_email", email)
		c.Next()
	}
}

// RequireCapability gates a route on one capability.
//
//	admin.GET("/users/:id/contact",
//	    middleware.RequireCapability(pool, models.CapViewUserContact),
//	    handlers.GetUserContactHandler(pool))
func RequireCapability(pool *pgxpool.Pool, cap models.Capability) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := StaffRoleFrom(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "staff access required"})
			return
		}
		if !models.Can(&role, cap) {
			email, _ := c.Get("actor_email")
			emailStr, _ := email.(string)
			auditDenied(pool, c, emailStr, &role, string(cap), "capability not held")

			// The message names the capability rather than saying "forbidden".
			// A staff member who can't do something needs to know what to ask
			// for; a vague refusal produces a support ticket instead.
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":               "you do not have permission for this action",
				"required_capability": string(cap),
				"your_role":           string(role),
			})
			return
		}
		c.Next()
	}
}

// StaffRoleFrom pulls the role set by RequireStaff.
func StaffRoleFrom(c *gin.Context) (models.StaffRole, bool) {
	v, ok := c.Get(ctxStaffRole)
	if !ok {
		return "", false
	}
	role, ok := v.(models.StaffRole)
	return role, ok
}

// IsLegacyAdmin reports whether the caller is a pre-staff-model platform admin.
//
// Used where "admin" is not a strong enough answer — identity documents, in
// particular, must never be reachable by an account that simply never had a
// role assigned.
func IsLegacyAdmin(c *gin.Context) bool {
	v, ok := c.Get(ctxStaffLegacy)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// RequireCapabilityOn gates a route that is NOT mounted under RequireStaff.
//
// The legacy admin endpoints — events, merch, collections, users, organisers,
// payouts — were registered before the staff model existed and are still on the
// bare admin group. RequireStaffWrite gates their mutating verbs at a single
// coarse level (CapApproveEvent), which produces two wrong answers at once:
//
//	an `access` invitee can GET every organiser's payout profile, because
//	reads are not gated at all
//
//	an `editor` can DELETE a user and rewrite payout bank details, because
//	one capability covers every write
//
// This middleware gives those routes a real capability each, while preserving
// the property that makes RequireStaffWrite safe to have deployed: an account
// with NO staff_role is a pre-existing platform admin and passes through
// untouched. Locking those accounts out is a regression, not a fix.
//
// The role is read from the database per request, for the same reason
// RequireStaff does: revoking access during an incident has to take effect now,
// not when a JWT expires.
func RequireCapabilityOn(pool *pgxpool.Pool, cap models.Capability) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Already resolved by RequireStaff further up the chain? Use that
		// rather than issuing a second identical query.
		if role, ok := StaffRoleFrom(c); ok {
			if models.Can(&role, cap) {
				c.Next()
				return
			}
			email, _ := c.Get("actor_email")
			emailStr, _ := email.(string)
			auditDenied(pool, c, emailStr, &role, string(cap), "capability not held")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":               "you do not have permission for this action",
				"required_capability": string(cap),
				"your_role":           string(role),
			})
			return
		}

		userID, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var roleStr *string
		var email string
		if err := pool.QueryRow(ctx,
			`SELECT staff_role::text, email FROM users WHERE id = $1`, userID,
		).Scan(&roleStr, &email); err != nil {
			log.Error().Err(err).Msg("capability gate: lookup failed")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}
		c.Set("actor_email", email)

		// Pre-existing platform admin: unchanged behaviour, deliberately.
		if roleStr == nil || *roleStr == "" {
			c.Next()
			return
		}

		role, err := models.ParseStaffRole(*roleStr)
		if err != nil {
			log.Error().Str("role", *roleStr).Msg("capability gate: unknown staff role")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "staff access required"})
			return
		}
		if !models.Can(&role, cap) {
			auditDenied(pool, c, email, &role, string(cap), "capability not held")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":               "you do not have permission for this action",
				"required_capability": string(cap),
				"your_role":           string(role),
			})
			return
		}
		c.Set(ctxStaffRole, role)
		c.Next()
	}
}

// auditDenied records a refused attempt.
//
// Fire-and-forget on its own context: the request is already being rejected,
// and a slow or failing audit write must not turn a clean 403 into a hang. The
// trade is that a denial can be lost if the database is down — acceptable,
// because the action was refused either way and the log is a detection aid
// rather than the control itself.
func auditDenied(pool *pgxpool.Pool, c *gin.Context, email string, role *models.StaffRole, action, reason string) {
	// Everything read from the request is captured HERE, on the request
	// goroutine. Gin recycles *gin.Context through a sync.Pool: once the handler
	// chain returns, c.Request is reassigned and c.Keys is rebuilt for the next
	// request. Reading them inside the goroutine raced that reuse, which both
	// attributed audit rows to the wrong caller and risked a
	// "concurrent map read and map write" runtime fatal — which no recover()
	// catches, including gin.Recovery().
	var roleStr *string
	if role != nil {
		s := string(*role)
		roleStr = &s
	}
	userID, _ := c.Get("user_id")
	path := c.Request.URL.Path
	ip := c.ClientIP()
	ua := c.Request.UserAgent()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err := pool.Exec(ctx, `
			INSERT INTO staff_audit_log
				(actor_id, actor_email, actor_role, action, allowed, detail, ip, user_agent)
			VALUES ($1, $2, $3::console_role, $4, FALSE,
			        jsonb_build_object('reason', $5::text, 'path', $6::text), $7, $8)
		`, userID, email, roleStr, action, reason, path, ip, ua)
		if err != nil {
			log.Warn().Err(err).Str("action", action).Msg("audit: denied-write failed")
		}
	}()
}

// AuditAllowed records a permitted sensitive action.
//
// Called by HANDLERS rather than middleware, because only the handler knows
// what was actually touched — middleware sees a route, not a subject. Viewing
// PII is audited as well as changing it: "who looked at this organiser's
// details" is a question that gets asked during an incident.
func AuditAllowed(pool *pgxpool.Pool, c *gin.Context, action, targetType string, targetID any, detail map[string]any) {
	email, _ := c.Get("actor_email")
	emailStr, _ := email.(string)
	userID, _ := c.Get("user_id")

	var roleStr *string
	if role, ok := StaffRoleFrom(c); ok {
		s := string(role)
		roleStr = &s
	}
	if detail == nil {
		detail = map[string]any{}
	}

	// Captured on the request goroutine — see the note in auditDenied.
	ip := c.ClientIP()
	ua := c.Request.UserAgent()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := pool.Exec(ctx, `
			INSERT INTO staff_audit_log
				(actor_id, actor_email, actor_role, action, target_type, target_id,
				 allowed, detail, ip, user_agent)
			VALUES ($1, $2, $3::console_role, $4, $5, $6, TRUE, $7, $8, $9)
		`, userID, emailStr, roleStr, action, targetType, targetID,
			detail, ip, ua)
		if err != nil {
			log.Warn().Err(err).Str("action", action).Msg("audit: write failed")
		}
	}()
}

// RequireStaffWrite gates mutating requests on the whole admin group.
//
// Registered once, after AdminOnlyMiddleware:
//
//	admin.Use(middleware.RequireStaffWrite(pool))
//
// Applied as a group middleware rather than per route so a destructive endpoint
// added later is gated by default instead of relying on someone remembering.
//
// WHAT THIS IS NOW: a read-only backstop, not the write policy.
//
// It used to gate every write on CapApproveEvent, which had a consequence
// nobody intended. The `access` role is described to the person granting it as
// "can approve initial KYC" and holds CapVerifyKYC — and could never use it,
// because this middleware refused the POST before the capability check ran. The
// role's headline permission was dead on arrival, and the only visible symptom
// was a 403 saying "your access level is read-only" to someone whose access
// level was not read-only.
//
// Per-route capabilities now do the real work (see RequireCapabilityOn). This
// stays as the floor: a staff member who holds NO write capability at all
// cannot mutate anything, including by calling the API directly with curl,
// which is the hole a UI-only check would leave open. It also still guards any
// write endpoint added later that someone forgets to gate.
//
// Two paths, and the split matters for not breaking existing accounts:
//
//   - staff_role IS NULL: a pre-existing platform admin. Allowed through
//     unchanged, because these accounts predate the staff model and locking
//     them out of their own console would be a regression.
//   - staff_role set: an invited console user. Refused only if the role holds
//     none of the write capabilities — which is exactly read_only.
func RequireStaffWrite(pool *pgxpool.Pool) gin.HandlerFunc {
	// Every capability that authorises a mutation. A role holding at least one
	// gets past this floor; which mutation it may actually perform is settled
	// per route.
	writeCaps := []models.Capability{
		models.CapVerifyKYC,
		models.CapEditEvents,
		models.CapApproveEvent,
		models.CapFeatureEvent,
		models.CapEditUserContact,
		models.CapRequestOTP,
		models.CapResetCredentials,
		models.CapRequestDangerous,
		models.CapActDangerous,
		models.CapApproveRequests,
		models.CapManageStaff,
	}
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		userID, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var roleStr *string
		var email string
		if err := pool.QueryRow(ctx,
			`SELECT staff_role::text, email FROM users WHERE id = $1`, userID,
		).Scan(&roleStr, &email); err != nil {
			log.Error().Err(err).Msg("staff write gate: lookup failed")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}

		if roleStr == nil || *roleStr == "" {
			c.Next()
			return
		}

		role, err := models.ParseStaffRole(*roleStr)
		if err != nil {
			// Unrecognised value: the enum grew and this binary is older than
			// the database. Denying is the only safe reading.
			auditDenied(pool, c, email, nil, "admin.write", "unknown staff role")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":     "your access level is read-only",
				"your_role": *roleStr,
			})
			return
		}

		canWriteSomething := false
		for _, wc := range writeCaps {
			if models.Can(&role, wc) {
				canWriteSomething = true
				break
			}
		}
		if !canWriteSomething {
			auditDenied(pool, c, email, &role, "admin.write", "write attempted without permission")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":     "your access level is read-only",
				"your_role": *roleStr,
			})
			return
		}
		c.Next()
	}
}