package controllers

import (
	"events-db/internal/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
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
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "User not found or database error",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "User successfully promoted to admin",
			"user":    user,
		})
	}
}

func DemoteUserHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		targetUserID := c.Param("id")

		query := `
			UPDATE users
			SET role = 'user', updated_at = $2
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
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "User not found or database error",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "User demoted to regular user",
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
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count users"})
			return
		}

		offset := (page - 1) * limit

		query := `
			SELECT id, email, role, created_at
			FROM users
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