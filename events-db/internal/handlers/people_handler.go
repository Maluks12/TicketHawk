package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Capability-scoped people listings
//
// THE SERVER REDACTS. THE CLIENT ONLY DECIDES WHAT TO DRAW.
//
// The obvious implementation is to keep the existing /admin/users endpoint and
// have the console hide the columns a role shouldn't see. That is not a control
// — it is a curtain. The payload still contains every organiser's email, and
// anyone who opens devtools, reads the network tab, or calls the endpoint with
// curl gets the lot regardless of their role.
//
// So the SHAPE of the response changes with the caller's capabilities. A
// read_only staff member's response does not contain an email field at all —
// not an empty one, not a masked one. There is nothing to un-hide.
//
// The cost is that the frontend can't assume a fixed shape and has to treat
// every gated field as optional. That is the correct trade: an optional field
// is a small inconvenience, a leaked email list is a POPIA problem.
// ─────────────────────────────────────────────────────────────────────────────

// scopedUser is built per-request. Every gated field is a POINTER so it can be
// omitted entirely rather than serialised as "" — `email: null` and no email
// key at all say different things, and only the latter means "you may not see
// this".
type scopedUser struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	Role        string  `json:"role"`
	StaffRole   *string `json:"staff_role,omitempty"`

	// Requires user.contact
	Email *string `json:"email,omitempty"`
	Phone *string `json:"phone,omitempty"`

	// Requires kyc.status
	KYCLevel  *int    `json:"kyc_level,omitempty"`
	KYCStatus *string `json:"kyc_status,omitempty"`

	// Requires bank.view_tail. Only ever the last four digits — the full number
	// is not in this database at all; it lives with the payment provider.
	BankTail *string `json:"bank_tail,omitempty"`
	BankName *string `json:"bank_name,omitempty"`

	IsOrganiser bool `json:"is_organiser"`
}

// GET /admin/people?tab=users|staff&q=&limit=
//
// Requires CapViewUserList (every staff role). Everything beyond names and
// roles is added only if the caller holds the matching capability.
func GetScopedPeopleHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := middleware.StaffRoleFrom(c)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "staff access required"})
			return
		}

		canContact := models.Can(&role, models.CapViewUserContact)
		canBank := models.Can(&role, models.CapViewBankTail)
		canKYC := models.Can(&role, models.CapViewKYCStatus)

		tab := c.DefaultQuery("tab", "users")
		if tab == "staff" && !models.Can(&role, models.CapViewStaff) {
			// Audited: someone asking for the staff roster without permission is
			// worth knowing about even though it's refused.
			middleware.AuditAllowed(pool, c, "staff.list_denied", "staff", nil, nil)
			c.JSON(http.StatusForbidden, gin.H{
				"error":               "you do not have permission to view staff",
				"required_capability": string(models.CapViewStaff),
			})
			return
		}

		q := strings.TrimSpace(c.Query("q"))
		limit := 200

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		// One query for both tabs, differing only in the staff predicate.
		//
		// Search is applied over email even when the caller cannot SEE emails.
		// That is deliberate and worth being explicit about: a support agent
		// who is given an address by a customer needs to find the account, and
		// matching without displaying leaks one bit ("this address exists")
		// rather than the whole roster. If that bit matters to you, gate the
		// `q` parameter on user.contact — it is a one-line change here.
		staffFilter := "u.staff_role IS NULL"
		if tab == "staff" {
			staffFilter = "u.staff_role IS NOT NULL"
		}

		rows, err := pool.Query(ctx, `
			SELECT
				u.id::text,
				COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.first_name, u.last_name)), ''), '') AS name,
				u.role::text,
				u.staff_role::text,
				u.email,
				u.phone_number,
				o.kyc_level,
				o.kyc_status::text,
				o.bank_account_last_4,
				o.settlement_bank_name,
				(o.id IS NOT NULL) AS is_organiser
			FROM users u
			LEFT JOIN organizers o ON o.user_id = u.id
			WHERE `+staffFilter+`
			  AND ($1::text = '' OR u.email ILIKE '%'||$1||'%'
			       OR TRIM(CONCAT_WS(' ', u.first_name, u.last_name)) ILIKE '%'||$1||'%')
			ORDER BY u.created_at DESC
			LIMIT $2
		`, q, limit)
		if err != nil {
			utils.InternalError(c, err, "could not load people")
			return
		}
		defer rows.Close()

		out := []scopedUser{}
		for rows.Next() {
			var (
				id, roleStr                       string
				name                              string
				staffRole, email, phone           *string
				kycStatus, bankTail, bankName     *string
				kycLevel                          *int
				isOrganiser                       bool
			)
			if err := rows.Scan(&id, &name, &roleStr, &staffRole, &email, &phone,
				&kycLevel, &kycStatus, &bankTail, &bankName, &isOrganiser); err != nil {
				utils.InternalError(c, err, "could not read people")
				return
			}

			su := scopedUser{
				ID:          id,
				Role:        roleStr,
				StaffRole:   staffRole,
				IsOrganiser: isOrganiser,
			}

			// A display name that never leaks the email.
			//
			// Plenty of accounts have no first/last name, and the tempting
			// fallback is the email — which would hand every address to
			// read_only through the one field nobody thought to gate. The
			// fallback is a short id instead: enough to tell two rows apart and
			// to quote in a support thread, and nothing more.
			switch {
			case name != "":
				su.DisplayName = name
			case canContact && email != nil:
				su.DisplayName = *email
			default:
				su.DisplayName = "Account " + id[:8]
			}

			if canContact {
				su.Email = email
				su.Phone = phone
			}
			if canKYC {
				su.KYCLevel = kycLevel
				su.KYCStatus = kycStatus
			}
			if canBank {
				su.BankTail = bankTail
				su.BankName = bankName
			}

			out = append(out, su)
		}
		if err := rows.Err(); err != nil {
			utils.InternalError(c, err, "could not read people")
			return
		}

		// Viewing PII is audited, not just changing it. "Who pulled the user
		// list with contact details" is a question that gets asked during an
		// incident, and it is unanswerable if only writes are recorded.
		if canContact {
			middleware.AuditAllowed(pool, c, "user.view_pii", "user_list", nil, map[string]any{
				"tab": tab, "count": len(out), "query": q != "",
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"people": out,
			"tab":    tab,
			// Echoed so the console renders the columns it was actually sent
			// rather than guessing from the role — same reasoning as
			// /me/capabilities.
			"fields": gin.H{
				"contact": canContact,
				"bank":    canBank,
				"kyc":     canKYC,
			},
		})
	}
}

// PUT /admin/staff/:id/role  { "staff_role": "editor" | null }
//
// Requires CapManageStaff (admin only).
func SetStaffRoleHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)
		targetID := c.Param("id")

		var body struct {
			StaffRole *string `json:"staff_role"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		// Revoking (null) is always allowed; granting must name a real role.
		if body.StaffRole != nil {
			if _, err := models.ParseStaffRole(*body.StaffRole); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "unknown staff role"})
				return
			}
		}

		// You cannot change your own staff role — in either direction.
		//
		// Self-promotion is the obvious hole. Self-DEMOTION matters too: the
		// last admin removing their own access locks everyone out of the
		// console permanently, and there is no recovery path short of psql.
		if targetID == actorID {
			c.JSON(http.StatusConflict, gin.H{
				"error": "you cannot change your own staff role — ask another admin",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// Removing the last admin is refused. Four-eyes already means one admin
		// can approve nothing; zero admins means nobody can grant a role back.
		if body.StaffRole == nil || *body.StaffRole != string(models.StaffAdmin) {
			var admins int
			if err := pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM users WHERE staff_role = 'admin' AND id <> $1`,
				targetID,
			).Scan(&admins); err != nil {
				utils.InternalError(c, err, "could not verify admin count")
				return
			}
			if admins == 0 {
				c.JSON(http.StatusConflict, gin.H{
					"error": "this is the last admin — grant someone else admin before removing this one",
				})
				return
			}
		}

		tag, err := pool.Exec(ctx,
			`UPDATE users SET staff_role = $2::console_role WHERE id = $1`,
			targetID, body.StaffRole)
		if err != nil {
			utils.InternalError(c, err, "could not update staff role")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		granted := "revoked"
		if body.StaffRole != nil {
			granted = *body.StaffRole
		}
		middleware.AuditAllowed(pool, c, "staff.role_change", "user", targetID, map[string]any{
			"new_role": granted,
		})

		c.JSON(http.StatusOK, gin.H{"staff_role": body.StaffRole})
	}
}