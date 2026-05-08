package handlers

import (
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Public read ────────────────────────────────────────────────────────────────

// GetCollectionByIDHandler returns a single active collection with its merch items.
// Route: GET /v1/collections/:id
func GetCollectionByIDHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		collectionID := c.Param("id")

		collection, err := repository.GetCollectionByID(pool, collectionID)
		if err != nil {
			if err == repository.ErrCollectionNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
				return
			}
			utils.InternalError(c, err, "get collection by id")
			return
		}

		c.JSON(http.StatusOK, collection)
	}
}

// ── Organiser write ────────────────────────────────────────────────────────────

// OrgCreateCollectionHandler creates a new collection owned by the organiser.
// Route: POST /v1/organiser/collections
func OrgCreateCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")

		var dto models.CreateCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		collection, err := repository.CreateCollection(pool, &organizerID, &dto)
		if err != nil {
			utils.InternalError(c, err, "org create collection")
			return
		}

		c.JSON(http.StatusCreated, collection)
	}
}

// OrgUpdateCollectionHandler updates a collection the organiser owns.
// Route: PUT /v1/organiser/collections/:id
func OrgUpdateCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		collectionID := c.Param("id")

		var dto models.UpdateCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		collection, err := repository.UpdateCollection(pool, collectionID, &organizerID, &dto)
		if err != nil {
			if err == repository.ErrCollectionNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found or access denied"})
				return
			}
			if err.Error() == "no fields to update" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No fields provided to update"})
				return
			}
			utils.InternalError(c, err, "org update collection")
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Collection updated", "collection": collection})
	}
}

// OrgDeactivateCollectionHandler soft-deletes a collection the organiser owns.
// Merchandise items in the collection are NOT deleted — their collection_id is
// set to NULL by the ON DELETE SET NULL FK constraint.
// Route: DELETE /v1/organiser/collections/:id
func OrgDeactivateCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		collectionID := c.Param("id")

		err := repository.DeactivateCollection(pool, collectionID, &organizerID)
		if err != nil {
			if err == repository.ErrCollectionNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found or access denied"})
				return
			}
			utils.InternalError(c, err, "org deactivate collection")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Collection removed",
			"note":    "Merchandise items in this collection have been un-assigned, not deleted",
		})
	}
}

// OrgGetCollectionsHandler returns all active collections owned by the organiser.
// Route: GET /v1/organiser/collections
func OrgGetCollectionsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")

		collections, err := repository.GetCollectionsByOrganizer(pool, organizerID)
		if err != nil {
			utils.InternalError(c, err, "org get collections")
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": collections, "count": len(collections)})
	}
}

// OrgAssignMerchToCollectionHandler assigns or un-assigns a merch item to a collection.
// Body: { "collection_id": "<uuid>" }  — pass null to un-assign.
// The merch item must belong to an event owned by the organiser.
// Route: PUT /v1/organiser/merch/:id/collection
func OrgAssignMerchToCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		merchID := c.Param("id")

		var dto models.AssignCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err := repository.AssignMerchToCollection(pool, merchID, dto.CollectionID, &organizerID)
		if err != nil {
			switch err {
			case repository.ErrCollectionNotFound:
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found or access denied"})
			case repository.ErrMerchNotFound:
				c.JSON(http.StatusNotFound, gin.H{"error": "Merchandise not found or access denied"})
			default:
				utils.InternalError(c, err, "org assign merch to collection")
			}
			return
		}

		msg := "Merchandise assigned to collection"
		if dto.CollectionID == nil {
			msg = "Merchandise removed from collection"
		}
		c.JSON(http.StatusOK, gin.H{"message": msg})
	}
}

// ── Admin handlers ─────────────────────────────────────────────────────────────

// AdminCreateCollectionHandler creates a collection with no organiser ownership.
// Route: POST /v1/admin/collections
func AdminCreateCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.CreateCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// nil organizerID — admin-owned collection
		collection, err := repository.CreateCollection(pool, nil, &dto)
		if err != nil {
			utils.InternalError(c, err, "admin create collection")
			return
		}

		c.JSON(http.StatusCreated, collection)
	}
}

// AdminUpdateCollectionHandler updates any collection regardless of ownership.
// Route: PUT /v1/admin/collections/:id
func AdminUpdateCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		collectionID := c.Param("id")

		var dto models.UpdateCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// nil organizerID — skip ownership check
		collection, err := repository.UpdateCollection(pool, collectionID, nil, &dto)
		if err != nil {
			if err == repository.ErrCollectionNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
				return
			}
			if err.Error() == "no fields to update" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No fields provided to update"})
				return
			}
			utils.InternalError(c, err, "admin update collection")
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Collection updated", "collection": collection})
	}
}

// AdminDeactivateCollectionHandler soft-deletes any collection.
// Route: DELETE /v1/admin/collections/:id
func AdminDeactivateCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		collectionID := c.Param("id")

		// nil organizerID — no ownership check
		err := repository.DeactivateCollection(pool, collectionID, nil)
		if err != nil {
			if err == repository.ErrCollectionNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
				return
			}
			utils.InternalError(c, err, "admin deactivate collection")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Collection removed",
			"note":    "Merchandise items in this collection have been un-assigned, not deleted",
		})
	}
}

// AdminGetCollectionsHandler returns all collections (active + inactive) for the dashboard.
// Route: GET /v1/admin/collections
func AdminGetCollectionsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		collections, err := repository.GetAllCollections(pool)
		if err != nil {
			utils.InternalError(c, err, "admin get collections")
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": collections, "count": len(collections)})
	}
}

// AdminAssignMerchToCollectionHandler assigns or un-assigns any merch item to any collection.
// No ownership checks — admin has full access.
// Route: PUT /v1/admin/merch/:id/collection
func AdminAssignMerchToCollectionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		merchID := c.Param("id")

		var dto models.AssignCollectionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// nil organizerID — admin, no ownership check
		err := repository.AssignMerchToCollection(pool, merchID, dto.CollectionID, nil)
		if err != nil {
			switch err {
			case repository.ErrCollectionNotFound:
				c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
			case repository.ErrMerchNotFound:
				c.JSON(http.StatusNotFound, gin.H{"error": "Merchandise not found"})
			default:
				utils.InternalError(c, err, "admin assign merch to collection")
			}
			return
		}

		msg := "Merchandise assigned to collection"
		if dto.CollectionID == nil {
			msg = "Merchandise removed from collection"
		}
		c.JSON(http.StatusOK, gin.H{"message": msg})
	}
}