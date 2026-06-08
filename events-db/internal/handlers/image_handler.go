package handlers

import (
	"errors"
	"events-db/internal/repository"
	"events-db/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Cover image ───────────────────────────────────────────────────────────────

func UploadCoverImageHandler(pool *pgxpool.Pool, imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		existingCover, _, _, _, err := repository.GetEventImageURLs(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found or you do not own this event"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch event"})
			return
		}

		url, err := imgService.UploadEventImage(file, header, eventID, services.ImageTypeCover)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.UpdateEventCoverImage(pool, eventID, organizerID.(string), url); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save image URL"})
			return
		}

		if existingCover != nil && *existingCover != "" {
			_ = imgService.DeleteEventImage(*existingCover)
		}

		c.JSON(http.StatusOK, gin.H{
			"message":         "Cover image uploaded successfully",
			"cover_image_url": url,
		})
	}
}

// ── Banner image ──────────────────────────────────────────────────────────────

func UploadBannerImageHandler(pool *pgxpool.Pool, imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		_, existingBanner, _, _, err := repository.GetEventImageURLs(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found or you do not own this event"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch event"})
			return
		}

		url, err := imgService.UploadEventImage(file, header, eventID, services.ImageTypeBanner)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.UpdateEventBannerImage(pool, eventID, organizerID.(string), url); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save image URL"})
			return
		}

		if existingBanner != nil && *existingBanner != "" {
			_ = imgService.DeleteEventImage(*existingBanner)
		}

		c.JSON(http.StatusOK, gin.H{
			"message":          "Banner image uploaded successfully",
			"banner_image_url": url,
		})
	}
}

// ── Banner video ──────────────────────────────────────────────────────────────

// UploadBannerVideoHandler accepts a multipart video file (MP4, MOV, or WebM,
// up to 500 MB) and stores it as the event's optional banner video. The banner
// image is not replaced — both can coexist. The frontend should use the video
// as the primary media and fall back to the banner image as a poster frame.
//
// Form field name: "video"
func UploadBannerVideoHandler(pool *pgxpool.Pool, imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		file, header, err := c.Request.FormFile("video")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "video file is required (field name: video)"})
			return
		}
		defer file.Close()

		_, _, existingVideo, _, err := repository.GetEventImageURLs(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found or you do not own this event"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch event"})
			return
		}

		url, err := imgService.UploadEventVideo(file, header, eventID, services.VideoTypeBanner)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.UpdateEventBannerVideo(pool, eventID, organizerID.(string), url); err != nil {
			// DB write failed — clean up the just-uploaded R2 object so we
			// don't leave orphaned files.
			_ = imgService.DeleteEventImage(url)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save video URL"})
			return
		}

		// Delete the old video from R2 after the DB row is committed.
		if existingVideo != nil && *existingVideo != "" {
			_ = imgService.DeleteEventImage(*existingVideo)
		}

		c.JSON(http.StatusOK, gin.H{
			"message":          "Banner video uploaded successfully",
			"banner_video_url": url,
		})
	}
}

// DeleteBannerVideoHandler removes the banner video from both R2 and the DB.
// The banner image is unaffected.
func DeleteBannerVideoHandler(pool *pgxpool.Pool, imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		_, _, existingVideo, _, err := repository.GetEventImageURLs(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found or you do not own this event"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch event"})
			return
		}

		if existingVideo == nil || *existingVideo == "" {
			c.JSON(http.StatusOK, gin.H{"message": "no banner video to delete"})
			return
		}

		// Clear the DB column first, then delete from R2. If R2 fails the URL
		// is already gone from the DB so it won't be served again.
		if err := repository.UpdateEventBannerVideo(pool, eventID, organizerID.(string), ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove video reference"})
			return
		}

		if err := imgService.DeleteEventImage(*existingVideo); err != nil {
			// Log-worthy but not fatal from the client's perspective — the URL
			// is already cleared in the DB.
			c.JSON(http.StatusOK, gin.H{
				"message": "Banner video removed (R2 cleanup may be pending)",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Banner video deleted successfully"})
	}
}

// ── Gallery images ────────────────────────────────────────────────────────────

func UploadGalleryImageHandler(pool *pgxpool.Pool, imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		// Confirm event exists and organizer owns it. The gallery count is
		// enforced atomically inside AppendGalleryImage so we don't need to
		// check it here — we only need the ownership check.
		_, _, _, _, err = repository.GetEventImageURLs(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found or you do not own this event"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch event"})
			return
		}

		url, err := imgService.UploadEventImage(file, header, eventID, services.ImageTypeGallery)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.AppendGalleryImage(pool, eventID, organizerID.(string), url); err != nil {
			// Upload succeeded but DB update failed — attempt R2 cleanup so we
			// don't leave orphaned files.
			_ = imgService.DeleteEventImage(url)

			if errors.Is(err, repository.ErrGalleryFull) {
				c.JSON(http.StatusConflict, gin.H{
					"error": "Gallery is full: a maximum of 10 images are allowed per event",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save gallery image"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":   "Gallery image uploaded successfully",
			"image_url": url,
		})
	}
}

// ── Collection cover image ────────────────────────────────────────────────────

// UploadCollectionCoverImageHandler handles POST /admin/collections/:id/image
// and POST /organiser/collections/:id/image.
// It uploads the image to R2 and writes the URL into merch_collections.cover_image_url.
// For organiser routes, middleware sets "user_id" so ownership is enforced;
// for admin routes, organizerID is left nil to skip the ownership check.
func UploadCollectionCoverImageHandler(pool *pgxpool.Pool, imgService *services.ImageService, isAdmin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		collectionID := c.Param("id")
		if _, err := uuid.Parse(collectionID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection ID"})
			return
		}

		var organizerID *string
		if !isAdmin {
			uid, _ := c.Get("user_id")
			s := uid.(string)
			organizerID = &s
		}

		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		url, err := imgService.UploadCollectionImage(file, header, collectionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.UpdateCollectionCoverImage(pool, collectionID, organizerID, url); err != nil {
			if errors.Is(err, repository.ErrCollectionNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "collection not found or access denied"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save image URL"})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":           "Collection cover image uploaded successfully",
			"cover_image_url":   url,
		})
	}
}
// UploadAdminImageHandler handles POST /admin/upload.
// A generic image upload that stores to R2 and returns the public URL.
// No DB write — the caller (admin dashboard) associates the URL with an entity.
func UploadAdminImageHandler(imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		url, err := imgService.UploadGenericImage(file, header)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"image_url": url})
	}
}

// UploadOrganiserImageHandler handles POST /organiser/upload.
// A generic image upload for the organiser dashboard — stores to R2 and
// returns the public URL with no DB write. The organiser dashboard uses this
// during event creation (before an event ID exists) so the URL can be
// included in the CreateEventDTO payload as cover_image_url / banner_image_url.
// Route is protected by AuthMiddleware + OrganizerOnlyMiddleware.
func UploadOrganiserImageHandler(imgService *services.ImageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		url, err := imgService.UploadGenericImage(file, header)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"image_url": url})
	}
}