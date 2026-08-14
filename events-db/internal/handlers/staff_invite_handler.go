package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Staff invitations.
//
// Accepting requires two things the invitee cannot both obtain alone: the
// emailed link, and an OTP that is delivered to the master admin and passed on
// out of band. A compromised mailbox is not enough.

// POST /admin/staff/invites  { "email": "...", "staff_role": "editor" }
// Requires CapManageStaff.
func CreateStaffInviteHandler(pool *pgxpool.Pool, sendInvite func(to, link, role string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)

		var body struct {
			Email     string `json:"email"`
			StaffRole string `json:"staff_role"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		if email == "" || !strings.Contains(email, "@") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a valid email is required"})
			return
		}
		role, err := models.ParseStaffRole(body.StaffRole)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown staff role"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		var existingStaff *string
		_ = pool.QueryRow(ctx,
			`SELECT staff_role::text FROM users WHERE lower(email) = $1`, email,
		).Scan(&existingStaff)
		if existingStaff != nil && *existingStaff != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "that person already has console access"})
			return
		}

		token, err := secureToken(32)
		if err != nil {
			utils.InternalError(c, err, "could not create invite")
			return
		}
		sum := sha256.Sum256([]byte(token))

		var id string
		err = pool.QueryRow(ctx, `
			INSERT INTO staff_invites (email, staff_role, invited_by, token_hash)
			VALUES ($1, $2::console_role, $3, $4)
			RETURNING id::text
		`, email, string(role), actorID, hex.EncodeToString(sum[:])).Scan(&id)
		if err != nil {
			// The partial unique index rejects a second open invite for the
			// same address rather than silently issuing two live tokens.
			c.JSON(http.StatusConflict, gin.H{"error": "there is already an open invite for that email"})
			return
		}

		link := strings.TrimRight(inviteBaseURL(c), "/") + "/staff/join?token=" + token
		if sendInvite != nil {
			if mailErr := sendInvite(email, link, string(role)); mailErr != nil {
				utils.InternalError(c, mailErr, "invite created but the email could not be sent")
				return
			}
		}

		middleware.AuditAllowed(pool, c, "staff.invite_created", "staff_invite", id, map[string]any{
			"email": email, "staff_role": string(role),
		})

		c.JSON(http.StatusCreated, gin.H{
			"id": id, "email": email, "staff_role": string(role),
			"expires_in_days": 7,
			// Returned so the admin can hand it over directly if mail is slow.
			"link": link,
		})
	}
}

// GET /admin/staff/invites
func ListStaffInvitesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		rows, err := pool.Query(ctx, `
			SELECT i.id::text, i.email, i.staff_role::text, i.status::text,
			       i.invited_at, i.expires_at, u.email
			FROM staff_invites i
			JOIN users u ON u.id = i.invited_by
			WHERE i.status IN ('pending', 'started')
			ORDER BY i.invited_at DESC
			LIMIT 100
		`)
		if err != nil {
			utils.InternalError(c, err, "could not list invites")
			return
		}
		defer rows.Close()

		type item struct {
			ID        string    `json:"id"`
			Email     string    `json:"email"`
			StaffRole string    `json:"staff_role"`
			Status    string    `json:"status"`
			InvitedAt time.Time `json:"invited_at"`
			ExpiresAt time.Time `json:"expires_at"`
			InvitedBy string    `json:"invited_by"`
		}
		out := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.Email, &it.StaffRole, &it.Status,
				&it.InvitedAt, &it.ExpiresAt, &it.InvitedBy); err != nil {
				utils.InternalError(c, err, "could not read invites")
				return
			}
			out = append(out, it)
		}
		c.JSON(http.StatusOK, gin.H{"invites": out})
	}
}

// DELETE /admin/staff/invites/:id
func RevokeStaffInviteHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		tag, err := pool.Exec(ctx, `
			UPDATE staff_invites SET status = 'revoked', revoked_at = now()
			WHERE id = $1 AND status IN ('pending', 'started')
		`, c.Param("id"))
		if err != nil {
			utils.InternalError(c, err, "could not revoke invite")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "invite not found or already used"})
			return
		}
		middleware.AuditAllowed(pool, c, "staff.invite_revoked", "staff_invite", c.Param("id"), nil)
		c.JSON(http.StatusOK, gin.H{"status": "revoked"})
	}
}

// ── Invitee-facing, token-authenticated ─────────────────────────────────────

// GET /staff/invite?token=...
func GetStaffInviteHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		inv, err := loadInvite(c, pool, c.Query("token"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "this invite link is invalid or has expired"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"email": inv.email, "staff_role": inv.role, "status": inv.status,
		})
	}
}

// POST /staff/invite/start  { "token": "..." }
//
// Issues the OTP and sends it to the master admin — never to the invitee. That
// split is what makes the second factor meaningful: the person accepting has to
// obtain it from the person granting access.
func StartStaffInviteHandler(pool *pgxpool.Pool, notifyAdmin func(to, code, invitee string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Token string `json:"token"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		inv, err := loadInvite(c, pool, body.Token)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "this invite link is invalid or has expired"})
			return
		}

		code, err := secureCode(6)
		if err != nil {
			utils.InternalError(c, err, "could not issue code")
			return
		}
		sum := sha256.Sum256([]byte(code))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		if _, err := pool.Exec(ctx, `
			UPDATE staff_invites
			SET status = 'started', otp_hash = $2, otp_issued_at = now(), otp_attempts = 0
			WHERE id = $1
		`, inv.id, hex.EncodeToString(sum[:])); err != nil {
			utils.InternalError(c, err, "could not issue code")
			return
		}

		// Master admins are the recipients. If none exists yet the invite
		// cannot be completed, which is the correct failure — nobody should be
		// able to self-grant console access.
		// Collect first, then send. Calling the mail provider inside an open
		// rows loop holds a pool connection for the duration of every SMTP
		// round trip.
		rows, err := pool.Query(ctx,
			`SELECT email FROM users WHERE is_master_admin = TRUE AND email <> ''`)
		if err != nil {
			utils.InternalError(c, err, "could not notify")
			return
		}
		admins := []string{}
		for rows.Next() {
			var to string
			if err := rows.Scan(&to); err == nil {
				admins = append(admins, to)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			utils.InternalError(c, err, "could not notify")
			return
		}

		// No master admin means nobody can receive the code, so the invite can
		// never be completed. Failing loudly here beats a 200 that leaves the
		// invitee waiting for an email that was never addressed to anyone.
		if len(admins) == 0 {
			c.JSON(http.StatusConflict, gin.H{
				"error": "no master admin is configured, so the confirmation code cannot be delivered",
				"hint":  "Set users.is_master_admin = TRUE for at least one account.",
			})
			return
		}

		sent, failed := 0, 0
		var lastErr error
		for _, to := range admins {
			if notifyAdmin == nil {
				break
			}
			if err := notifyAdmin(to, code, inv.email); err != nil {
				failed++
				lastErr = err
				continue
			}
			sent++
		}

		// The code is already stored, so a total send failure leaves an invite
		// nobody can finish. Report it rather than returning success.
		if sent == 0 {
			utils.InternalError(c, lastErr, "the confirmation code could not be emailed to any administrator")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"sent_to_admins": sent,
			"failed":         failed,
			"note":           "Ask the administrator who invited you for the 6-digit code.",
		})
	}
}

// POST /staff/invite/complete
//
//	{ "token": "...", "otp": "123456", "first_name": "...", "last_name": "...", "password": "..." }
func CompleteStaffInviteHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Token     string `json:"token"`
			OTP       string `json:"otp"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Password  string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if len(body.Password) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
			return
		}
		inv, err := loadInvite(c, pool, body.Token)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "this invite link is invalid or has expired"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctx)
		if err != nil {
			utils.InternalError(c, err, "could not complete invite")
			return
		}
		defer tx.Rollback(ctx)

		var storedHash *string
		var attempts int
		err = tx.QueryRow(ctx, `
			UPDATE staff_invites SET otp_attempts = otp_attempts + 1
			WHERE id = $1 AND status = 'started' AND otp_attempts < 5
			RETURNING otp_hash, otp_attempts
		`, inv.id).Scan(&storedHash, &attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusGone, gin.H{"error": "too many attempts, or no code has been issued yet"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "could not verify code")
			return
		}

		sum := sha256.Sum256([]byte(body.OTP))
		if storedHash == nil || *storedHash != hex.EncodeToString(sum[:]) {
			_ = tx.Commit(ctx) // keep the incremented attempt count
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "incorrect code", "attempts_remaining": 5 - attempts,
			})
			return
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			utils.InternalError(c, err, "could not set password")
			return
		}

		// An invitee may already hold an ordinary account. Upsert rather than
		// insert so console access is added to it instead of creating a second
		// account on the same address.
		var userID string
		err = tx.QueryRow(ctx, `
			INSERT INTO users (email, password, first_name, last_name, role, staff_role, email_verified)
			VALUES ($1, $2, $3, $4, 'attendee', $5::console_role, TRUE)
			ON CONFLICT (email) DO UPDATE SET
				staff_role = EXCLUDED.staff_role,
				first_name = COALESCE(NULLIF(users.first_name, ''), EXCLUDED.first_name),
				last_name  = COALESCE(NULLIF(users.last_name, ''), EXCLUDED.last_name)
			RETURNING id::text
		`, inv.email, string(hashed), body.FirstName, body.LastName, inv.role).Scan(&userID)
		if err != nil {
			utils.InternalError(c, err, "could not create the account")
			return
		}

		if _, err := tx.Exec(ctx, `
			UPDATE staff_invites
			SET status = 'accepted', accepted_at = now(), accepted_user = $2,
			    otp_hash = NULL
			WHERE id = $1
		`, inv.id, userID); err != nil {
			utils.InternalError(c, err, "could not finalise invite")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			utils.InternalError(c, err, "could not finalise invite")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "accepted", "email": inv.email, "staff_role": inv.role,
		})
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

type inviteRow struct {
	id, email, role, status string
}

func loadInvite(c *gin.Context, pool *pgxpool.Pool, token string) (*inviteRow, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("no token")
	}
	sum := sha256.Sum256([]byte(token))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var r inviteRow
	err := pool.QueryRow(ctx, `
		SELECT id::text, email, staff_role::text, status::text
		FROM staff_invites
		WHERE token_hash = $1 AND status IN ('pending', 'started') AND expires_at > now()
	`, hex.EncodeToString(sum[:])).Scan(&r.id, &r.email, &r.role, &r.status)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func inviteBaseURL(c *gin.Context) string {
	if origin := c.GetHeader("Origin"); origin != "" {
		return origin
	}
	return "https://admin.ticket-hawk.com"
}

func secureToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}