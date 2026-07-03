package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Event staffing endpoints ──────────────────────────────────────────────────
//
// Management endpoints (list/add/revoke staff, create/list/revoke codes) are
// gated by CanManageEventStaff: event owner, platform admin, or manager-role
// staff. Redemption and "my scannable events" are open to any authed user —
// scanning power comes from the membership row, not the global role.

// requireStaffManager centralises the manage-permission gate. Returns the
// eventID when allowed; writes the HTTP error and returns "" otherwise.
func requireStaffManager(c *gin.Context, ctx context.Context, pool *pgxpool.Pool) string {
	eventID := c.Param("id")
	if _, err := uuid.Parse(eventID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return ""
	}

	userID := c.MustGet("user_id").(string)
	role := c.GetString("role")

	ok, err := repository.CanManageEventStaff(ctx, pool, eventID, userID, role)
	if err != nil {
		if errors.Is(err, repository.ErrEventNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check permissions"})
		}
		return ""
	}
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have staff-management access for this event"})
		return ""
	}
	return eventID
}

// ListEventStaffHandler lists an event's staff (active + revoked).
//
//	GET /v1/events/:id/staff
func ListEventStaffHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		staff, err := repository.ListEventStaff(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load staff"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"staff": staff})
	}
}

// AddEventStaffHandler grants staff access to an existing account by email.
//
//	POST /v1/events/:id/staff   { email, role? }
func AddEventStaffHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var input struct {
			Email string `json:"email" binding:"required,email"`
			Role  string `json:"role"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email is required"})
			return
		}
		if input.Role != "manager" {
			input.Role = "scanner" // default + whitelist in one step
		}

		member, err := repository.AddStaffByEmail(ctx, pool, eventID, input.Email, input.Role, c.MustGet("user_id").(string))
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrUserNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "No TicketHawk account with that email. Ask them to register first, or use a join code."})
			case errors.Is(err, repository.ErrEventNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			default:
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Staff member added", "member": member})
	}
}

// RevokeEventStaffHandler soft-revokes a staffer's access.
//
//	DELETE /v1/events/:id/staff/:userId
func RevokeEventStaffHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		staffUserID := c.Param("userId")
		if _, err := uuid.Parse(staffUserID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		err := repository.RevokeStaff(ctx, pool, eventID, staffUserID, c.MustGet("user_id").(string))
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "No active staff member with that ID"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke access"})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Access revoked. Takes effect on their next scan."})
	}
}

// CreateStaffCodeHandler mints a join code for door staff.
//
//	POST /v1/events/:id/staff/codes   { role?, max_uses?, expires_at? }
func CreateStaffCodeHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var input struct {
			Role      string     `json:"role"`
			MaxUses   int        `json:"max_uses"`
			ExpiresAt *time.Time `json:"expires_at"`
		}
		_ = c.ShouldBindJSON(&input) // all fields optional; defaults below
		if input.Role != "manager" {
			input.Role = "scanner"
		}
		exp := time.Time{}
		if input.ExpiresAt != nil {
			exp = *input.ExpiresAt
		}

		code, err := repository.CreateStaffCode(ctx, pool, eventID, input.Role, input.MaxUses, exp, c.MustGet("user_id").(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create code"})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": code})
	}
}

// ListStaffCodesHandler lists an event's join codes with live use counts.
//
//	GET /v1/events/:id/staff/codes
func ListStaffCodesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		codes, err := repository.ListStaffCodes(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load codes"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"codes": codes})
	}
}

// RevokeStaffCodeHandler revokes a join code. Existing members stay (they're
// individually revocable from the staff list).
//
//	DELETE /v1/events/:id/staff/codes/:codeId
func RevokeStaffCodeHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		codeID := c.Param("codeId")
		if _, err := uuid.Parse(codeID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid code ID"})
			return
		}

		if err := repository.RevokeStaffCode(ctx, pool, eventID, codeID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Code not found or already revoked"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Code revoked. Staff who already joined keep access until individually removed."})
	}
}

// RedeemStaffCodeHandler lets any authed user redeem a join code and become
// staff for that code's event. Rate-limited at the route level; the error is
// deliberately uniform so a wrong code learns nothing.
//
//	POST /v1/staff/redeem   { code }
func RedeemStaffCodeHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			Code string `json:"code" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		ev, err := repository.RedeemStaffCode(ctx, pool, input.Code, c.MustGet("user_id").(string))
		if err != nil {
			if errors.Is(err, repository.ErrCodeInvalid) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired code"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to redeem code"})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "You can now scan tickets for this event", "event": ev})
	}
}

// ListMyScanEventsHandler returns the events the caller may scan — drives the
// staffer's event list and the scan screen picker.
//
//	GET /v1/staff/events
func ListMyScanEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		events, err := repository.ListScannableEvents(ctx, pool, c.MustGet("user_id").(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load events"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"events": events})
	}
}