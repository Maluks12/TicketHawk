package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"events-db/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func PromoteUserHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		targetUserID := c.Param("id")

		query := `
			UPDATE users
			SET role = 'admin', updated_at = $2
			WHERE id = $1
			RETURNING id, email, role, created_at
		`

		var user models.User
		err := pool.QueryRow(ctx, query, targetUserID, time.Now()).Scan(
			&user.ID,
			&user.Email,
			&user.Role,
			&user.CreatedAt,
		)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to promote user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "User successfully promoted to admin",
			"user":    user,
		})
	}
}

// DemoteUserHandler removes admin from an account.
//
// It used to write role = 'user'. There is no 'user' label in the user_role
// enum — it is ('attendee', 'organizer', 'admin') — so every demote failed with
// 22P02, and the handler returned it as a 404 "User not found or database
// error". Two of the three states the console can put an account in have never
// worked. This is the same shape as the organizer_status bug migration 000079
// fixed, and it hid for the same reason: a 404 on an admin action reads as a
// stale row, not as a missing enum label.
//
// The target is chosen rather than fixed. Demoting straight to 'attendee' would
// strip organiser access from an admin who also runs events, and every event
// and payout screen would start 403-ing the moment the demotion landed. An
// account with an organizers row therefore falls back to 'organizer'; everyone
// else to 'attendee'. The CASE runs inside the UPDATE, so there is no
// read-then-write race with a concurrent organiser registration.
func DemoteUserHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		targetUserID := c.Param("id")

		query := `
			UPDATE users u
			SET role = CASE
			             WHEN EXISTS (SELECT 1 FROM organizers o WHERE o.user_id = u.id)
			             THEN 'organizer'::user_role
			             ELSE 'attendee'::user_role
			           END,
			    updated_at = $2
			WHERE u.id = $1
			RETURNING u.id, u.email, u.role, u.created_at
		`

		var user models.User
		err := pool.QueryRow(ctx, query, targetUserID, time.Now()).Scan(
			&user.ID,
			&user.Email,
			&user.Role,
			&user.CreatedAt,
		)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}
			// Don't hand the raw driver error back to the console. Returning
			// err.Error() verbatim is how a broken enum label managed to look
			// like a missing user for months.
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to demote user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Admin access removed",
			"user":    user,
		})
	}
}

// add search functionality to user search
// add role filtering
func GetAllUsersHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		limitStr := c.DefaultQuery("limit", "5")
		pageStr := c.DefaultQuery("page", "1")

		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			limit = 5
		}

		page, err := strconv.Atoi(pageStr)
		if err != nil || page <= 0 {
			page = 1
		}

		var total int
		// Deleted accounts are anonymous stubs kept only so the bookings and
		// events foreign keys stay valid (migration 000065). They are not
		// users and do not belong in a user listing. The COUNT is filtered
		// alongside the SELECT below — filtering one and not the other would
		// hand the console a page count it can never fill.
		err = pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`).Scan(&total)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count users"})
			return
		}

		offset := (page - 1) * limit

		query := `
			SELECT id, email, role, created_at
			FROM users
			WHERE deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`

		rows, err := pool.Query(ctx, query, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
			return
		}
		defer rows.Close()

		var users []models.User
		for rows.Next() {
			var user models.User
			err := rows.Scan(
				&user.ID,
				&user.Email,
				&user.Role,
				&user.CreatedAt,
			)
			if err != nil {
				continue
			}
			users = append(users, user)
		}
		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
			return
		}

		totalPages := (total + limit - 1) / limit

		c.JSON(http.StatusOK, gin.H{
			"users":      users,
			"page":       page,
			"limit":      limit,
			"count":      len(users),
			"total":      total,
			"totalPages": totalPages,
		})
	}
}