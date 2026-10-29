package handlers

import (
	"context"
	"errors"
	"net/http"

	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Event collections (migration 000096)
//
// Organiser (auth + organiser role):
//
//	GET    /v1/organiser/event-collections              list mine
//	POST   /v1/organiser/event-collections              create (title, description, event_ids)
//	GET    /v1/organiser/event-collections/:id          one of mine
//	PATCH  /v1/organiser/event-collections/:id          title / description
//	DELETE /v1/organiser/event-collections/:id          delete (events are released, not deleted)
//	PUT    /v1/organiser/event-collections/:id/events   replace membership
//	POST   /v1/organiser/event-collections/:id/cover    upload cover (multipart, field "image")
//	DELETE /v1/organiser/event-collections/:id/cover    remove cover
//
// Public:
//
//	GET    /v1/event-collections/:slug                  collection + upcoming public dates
//
// Not /collections: that path belongs to MERCH collections (migration 000029).
//
// Every write busts the cached home feed, because the feed is where a
// collection changes what people see; the per-section event caches hold
// events, which a collection change does not alter.
// ─────────────────────────────────────────────────────────────────────────────

func bustHomeFeed(ctx context.Context, ec *cache.EventCache) {
	if ec != nil {
		ec.BustHome(ctx)
	}
}

func writeEventCollectionError(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, repository.ErrEventCollectionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
	case errors.Is(err, repository.ErrEventNotOwned):
		c.JSON(http.StatusBadRequest, gin.H{"error": "You can only add your own events to a collection"})
	default:
		utils.InternalError(c, err, op)
	}
}

func ListMyEventCollectionsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := repository.ListOrganizerEventCollections(c.Request.Context(), pool, c.GetString("user_id"))
		if err != nil {
			utils.InternalError(c, err, "listing event collections")
			return
		}
		c.JSON(http.StatusOK, gin.H{"collections": list})
	}
}

func GetMyEventCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		col, err := repository.GetOrganizerEventCollection(c.Request.Context(), pool, c.GetString("user_id"), c.Param("id"))
		if err != nil {
			writeEventCollectionError(c, err, "loading event collection")
			return
		}
		c.JSON(http.StatusOK, col)
	}
}

func CreateEventCollectionHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.CreateEventCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}
		col, err := repository.CreateEventCollection(c.Request.Context(), pool, c.GetString("user_id"), dto)
		if err != nil {
			writeEventCollectionError(c, err, "creating event collection")
			return
		}
		bustHomeFeed(c.Request.Context(), ec)
		c.JSON(http.StatusCreated, col)
	}
}

func UpdateEventCollectionHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.UpdateEventCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}
		col, err := repository.UpdateEventCollection(c.Request.Context(), pool, c.GetString("user_id"), c.Param("id"), dto)
		if err != nil {
			writeEventCollectionError(c, err, "updating event collection")
			return
		}
		bustHomeFeed(c.Request.Context(), ec)
		c.JSON(http.StatusOK, col)
	}
}

func DeleteEventCollectionHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := repository.DeleteEventCollection(c.Request.Context(), pool, c.GetString("user_id"), c.Param("id")); err != nil {
			writeEventCollectionError(c, err, "deleting event collection")
			return
		}
		bustHomeFeed(c.Request.Context(), ec)
		c.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

func SetEventCollectionEventsHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.SetEventCollectionEventsDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}
		col, err := repository.SetEventCollectionEvents(c.Request.Context(), pool, c.GetString("user_id"), c.Param("id"), dto.EventIDs)
		if err != nil {
			writeEventCollectionError(c, err, "setting event collection membership")
			return
		}
		bustHomeFeed(c.Request.Context(), ec)
		c.JSON(http.StatusOK, col)
	}
}

// UploadEventCollectionCoverHandler stores a cover and points the collection
// at it. The cover is ONLY settable this way, never by URL, so a collection can
// only show an image that went through our upload pipeline.
func UploadEventCollectionCoverHandler(pool *pgxpool.Pool, imgService *services.ImageService, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		id := c.Param("id")

		file, header, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required (field name: image)"})
			return
		}
		defer file.Close()

		// Check ownership BEFORE spending an upload.
		owned, err := repository.EventCollectionOwned(c.Request.Context(), pool, userID, id)
		if err != nil {
			utils.InternalError(c, err, "checking event collection ownership")
			return
		}
		if !owned {
			c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
			return
		}

		url, err := imgService.UploadGenericImage(file, header)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := repository.SetEventCollectionCover(c.Request.Context(), pool, userID, id, &url); err != nil {
			writeEventCollectionError(c, err, "saving event collection cover")
			return
		}
		bustHomeFeed(c.Request.Context(), ec)
		c.JSON(http.StatusOK, gin.H{"cover_image_url": url})
	}
}

func RemoveEventCollectionCoverHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := repository.SetEventCollectionCover(c.Request.Context(), pool, c.GetString("user_id"), c.Param("id"), nil); err != nil {
			writeEventCollectionError(c, err, "removing event collection cover")
			return
		}
		bustHomeFeed(c.Request.Context(), ec)
		c.JSON(http.StatusOK, gin.H{"cover_image_url": nil})
	}
}

// GetPublicEventCollectionHandler serves the collection page.
//
// A collection whose dates have all passed still resolves, with an empty
// events list, so a link shared during the tour does not turn into a 404 the
// day after the last date — the page says it has finished instead.
func GetPublicEventCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		col, err := repository.GetPublicEventCollection(c.Request.Context(), pool, c.Param("slug"))
		if err != nil {
			if errors.Is(err, repository.ErrEventCollectionNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
				return
			}
			utils.InternalError(c, err, "loading public event collection")
			return
		}
		c.JSON(http.StatusOK, col)
	}
}
