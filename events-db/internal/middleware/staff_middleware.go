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
			auditDenied(pool, c, email, nil, "staff.access", "not staff")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "staff access required"})
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

// auditDenied records a refused attempt.
//
// Fire-and-forget on its own context: the request is already being rejected,
// and a slow or failing audit write must not turn a clean 403 into a hang. The
// trade is that a denial can be lost if the database is down — acceptable,
// because the action was refused either way and the log is a detection aid
// rather than the control itself.
func auditDenied(pool *pgxpool.Pool, c *gin.Context, email string, role *models.StaffRole, action, reason string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		var roleStr *string
		if role != nil {
			s := string(*role)
			roleStr = &s
		}
		userID, _ := c.Get("user_id")

		_, err := pool.Exec(ctx, `
			INSERT INTO staff_audit_log
				(actor_id, actor_email, actor_role, action, allowed, detail, ip, user_agent)
			VALUES ($1, $2, $3::console_role, $4, FALSE,
			        jsonb_build_object('reason', $5::text, 'path', $6::text), $7, $8)
		`, userID, email, roleStr, action, reason,
			c.Request.URL.Path, c.ClientIP(), c.Request.UserAgent())
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

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := pool.Exec(ctx, `
			INSERT INTO staff_audit_log
				(actor_id, actor_email, actor_role, action, target_type, target_id,
				 allowed, detail, ip, user_agent)
			VALUES ($1, $2, $3::console_role, $4, $5, $6, TRUE, $7, $8, $9)
		`, userID, emailStr, roleStr, action, targetType, targetID,
			detail, c.ClientIP(), c.Request.UserAgent())
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
// Two paths, and the split matters for not breaking existing accounts:
//
//   - staff_role IS NULL: a pre-existing platform admin. Allowed through
//     unchanged, because these accounts predate the staff model and locking
//     them out of their own console would be a regression.
//   - staff_role set: an invited console user. Writes require CapApproveEvent,
//     so read_only and access staff can read the console but cannot change
//     anything — including by calling the API directly, which is the hole a
//     UI-only check would leave open.
func RequireStaffWrite(pool *pgxpool.Pool) gin.HandlerFunc {
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
		if err != nil || !models.Can(&role, models.CapApproveEvent) {
			auditDenied(pool, c, email, nil, "admin.write", "write attempted without permission")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":     "your access level is read-only",
				"your_role": *roleStr,
			})
			return
		}
		c.Next()
	}
}