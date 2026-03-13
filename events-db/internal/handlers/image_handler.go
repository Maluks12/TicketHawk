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

		existingCover, _, _, err := repository.GetEventImageURLs(pool, eventID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found or you do not own this event "})
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
			_=imgService.DeleteEventImage(*existingCover)
		}

		c.JSON(http.StatusOK, gin.H{
			"message":        "Cover image uploaded successfully",
			"cover_image_url": url,
		})
	}
}

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

		_, existingBanner, _, err := repository.GetEventImageURLs(pool, eventID, organizerID.(string))
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
 
		// Confirm event exists and organizer owns it
		_, _, _, err = repository.GetEventImageURLs(pool, eventID, organizerID.(string))
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
			// Upload succeeded but DB update failed — attempt R2 cleanup
			_ = imgService.DeleteEventImage(url)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save gallery image"})
			return
		}
 
		c.JSON(http.StatusOK, gin.H{
			"message":   "Gallery image uploaded successfully",
			"image_url": url,
		})
	}
}