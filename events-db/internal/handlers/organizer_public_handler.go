package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Public ───────────────────────────────────────────────────────────────────

// GetPublicOrganizerHandler serves an organiser's public profile.
//
//	GET /v1/organisers/:slug
//
// :slug accepts a slug or a UUID, matching the event detail route.
//
// Every failure to resolve — no such organiser, page disabled, account
// suspended, KYC below level 2 — returns an identical 404. See
// ErrOrganizerProfileNotFound for why the causes are not distinguished.
func GetPublicOrganizerHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")

		profile, err := repository.GetPublicOrganizerProfile(pool, slug)
		if err != nil {
			if errors.Is(err, repository.ErrOrganizerProfileNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
				return
			}
			utils.InternalError(c, err, "failed to load organiser profile")
			return
		}

		c.JSON(http.StatusOK, profile)
	}
}

// GetPublicOrganizerEventsHandler lists an organiser's events.
//
//	GET /v1/organisers/:slug/events?scope=upcoming|past&limit=20&cursor=…
//
// The profile is re-resolved here rather than trusting the caller to have hit
// the profile endpoint first. Skipping it would let anyone list a suspended or
// hidden organiser's catalogue by guessing the slug — the events are individually
// public, but the fact that a particular organiser runs them is what the gate
// protects.
func GetPublicOrganizerEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")

		profile, err := repository.GetPublicOrganizerProfile(pool, slug)
		if err != nil {
			if errors.Is(err, repository.ErrOrganizerProfileNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
				return
			}
			utils.InternalError(c, err, "failed to load organiser profile")
			return
		}

		scope := repository.ScopeUpcoming
		if c.Query("scope") == string(repository.ScopePast) {
			scope = repository.ScopePast
		}

		limit := 20
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
				limit = n
			}
		}

		var cursor *utils.Cursor
		if raw := c.Query("cursor"); raw != "" {
			cur, err := utils.DecodeCursor(raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
			cursor = cur
		}

		events, next, err := repository.ListPublicOrganizerEvents(pool, profile.ID, scope, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "failed to list organiser events")
			return
		}

		resp := gin.H{
			"data":  events,
			"scope": string(scope),
		}
		if next != nil {
			resp["next_cursor"] = utils.EncodeCursor(next)
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ── Owner-side (authenticated organiser) ─────────────────────────────────────

// GetMyPublicProfileHandler returns the organiser's own page as the dashboard
// needs to render it, including when it is not live.
//
//	GET /v1/organiser/profile
func GetMyPublicProfileHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		view, err := repository.GetOwnerProfile(pool, userID)
		if err != nil {
			if errors.Is(err, repository.ErrOrganizerNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser profile not found"})
				return
			}
			utils.InternalError(c, err, "failed to load profile")
			return
		}

		c.JSON(http.StatusOK, view)
	}
}

// UpdateMyPublicProfileHandler applies the organiser's edits to their page.
//
//	PATCH /v1/organiser/profile
//
// Note what is NOT editable here: business_name (and therefore the slug),
// business_email, business_phone. Those sit behind the existing organiser
// update path and, for the name, behind the deliberate decision in 000077 that
// a slug is assigned once. This endpoint is only the public presentation layer.
func UpdateMyPublicProfileHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		var dto models.UpdatePublicProfileDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		if err := repository.UpdatePublicProfile(pool, userID, &dto); err != nil {
			if errors.Is(err, repository.ErrOrganizerNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser profile not found"})
				return
			}
			utils.InternalError(c, err, "failed to update profile")
			return
		}

		view, err := repository.GetOwnerProfile(pool, userID)
		if err != nil {
			// The write succeeded; failing the request now would make the
			// client believe otherwise and retry a change already applied.
			c.JSON(http.StatusOK, gin.H{"updated": true})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

// ReplaceMyLinksHandler replaces the organiser's whole social/website link set.
//
//	PUT /v1/organiser/profile/links
//
// Validation errors come back as 400 with the offending platform named, because
// "instagram: link does not point at that platform" is something an organiser
// can fix and "invalid request" is not.
func ReplaceMyLinksHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		var dto models.ReplaceOrganizerLinksDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		links, err := models.ValidateOrganizerLinks(dto.Links)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		organizerID, err := repository.GetOrganizerIDForUser(pool, userID)
		if err != nil {
			if errors.Is(err, repository.ErrOrganizerNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser profile not found"})
				return
			}
			utils.InternalError(c, err, "failed to resolve organiser")
			return
		}

		if err := repository.ReplaceOrganizerLinks(pool, organizerID, links); err != nil {
			utils.InternalError(c, err, "failed to save links")
			return
		}

		c.JSON(http.StatusOK, gin.H{"links": links})
	}
}

// GetLinkPlatformsHandler lists the platforms the dashboard picker may offer,
// in canonical display order.
//
//	GET /v1/organiser/profile/link-platforms
//
// Served from the server rather than hardcoded in three frontends so that
// adding a platform is one deploy, not four.
func GetLinkPlatformsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		out := make([]string, 0, len(models.OrganizerLinkPlatforms))
		for _, p := range models.OrganizerLinkPlatforms {
			out = append(out, string(p))
		}
		c.JSON(http.StatusOK, gin.H{"platforms": out})
	}
}