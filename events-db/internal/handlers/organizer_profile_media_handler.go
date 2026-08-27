package handlers

import (
	"errors"
	"net/http"

	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Organiser profile media ──────────────────────────────────────────────────
//
// Cover and logo for the public organiser page.
//
// These deliberately do NOT live under /organiser/events/:id/media. That group
// is keyed on an event the organiser must own, and its handlers all verify
// ownership through GetEventImageURLs. A profile image has no event, so reusing
// that group would mean either inventing a fake event id or bypassing the
// ownership check that makes the group safe.
//
// UploadGenericImage is reused rather than adding an ImageType: it already
// enforces the MIME allow-list, the 10 MB ceiling and compression, and writes
// to uploads/misc/. The only thing a dedicated type would buy is a tidier R2
// prefix, which is not worth a second code path through the compressor.

// UploadOrganizerCoverHandler stores a new cover image and points the
// organiser's profile at it.
//
//	POST /v1/organiser/profile/media/cover   (multipart, field name: image)
//
// The old image is NOT deleted from R2. Two reasons: a delete that races a CDN
// cache leaves visitors with a broken hero for the cache lifetime, and an
// organiser who re-uploads by mistake has no way back if we have already
// destroyed the previous file. Orphaned objects are cheap; a broken public page
// is not.
func UploadOrganizerCoverHandler(pool *pgxpool.Pool, imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		// Resolve the organiser BEFORE spending an upload. A user without an
		// organiser row should not be able to push bytes into our bucket.
		if _, err := repository.GetOrganizerIDForUser(pool, userID); err != nil {
			if errors.Is(err, repository.ErrOrganizerNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser profile not found"})
				return
			}
			utils.InternalError(c, err, "failed to resolve organiser")
			return
		}

		url, err := imgService.UploadGenericImage(file, header)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.SetOrganizerCoverImage(pool, userID, url); err != nil {
			// The bytes are in R2 but the row was not updated. Return the URL
			// anyway so the client can retry the PATCH without re-uploading a
			// 10 MB file over a mobile connection.
			c.JSON(http.StatusOK, gin.H{
				"cover_image_url": url,
				"saved":           false,
				"error":           "image uploaded but profile update failed; retry saving",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"cover_image_url": url, "saved": true})
	}
}

// UploadOrganizerLogoHandler stores a new logo and points the profile at it.
//
//	POST /v1/organiser/profile/media/logo   (multipart, field name: image)
//
// Same non-deletion policy as the cover, for the same reasons — and more
// sharply here, because the logo is rendered on every event card the organiser
// has ever published, not just on their own page.
func UploadOrganizerLogoHandler(pool *pgxpool.Pool, imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		if _, err := repository.GetOrganizerIDForUser(pool, userID); err != nil {
			if errors.Is(err, repository.ErrOrganizerNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser profile not found"})
				return
			}
			utils.InternalError(c, err, "failed to resolve organiser")
			return
		}

		url, err := imgService.UploadGenericImage(file, header)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.SetOrganizerLogo(pool, userID, url); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"logo_url": url,
				"saved":    false,
				"error":    "image uploaded but profile update failed; retry saving",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"logo_url": url, "saved": true})
	}
}