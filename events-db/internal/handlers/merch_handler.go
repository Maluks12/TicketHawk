package handlers
 
import (
  "events-db/internal/models"
  "events-db/internal/repository"
  "net/http"
 
  "github.com/gin-gonic/gin"
  "github.com/jackc/pgx/v5/pgxpool"
)
 
// GetEventMerchHandler returns the full merch catalogue for an event.
func GetEventMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
  return func(c *gin.Context) {
    eventID := c.Param("id")
    items, err := repository.GetMerchandiseByEvent(pool, eventID, 0)
    if err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load merchandise"})
      return
    }
    c.JSON(http.StatusOK, gin.H{"data": items, "count": len(items)})
  }
}
// GetEventMerchPreviewHandler returns the first 4 items for the event details page.
func GetEventMerchPreviewHandler(pool *pgxpool.Pool) gin.HandlerFunc {
  return func(c *gin.Context) {
    eventID := c.Param("id")
    items, err := repository.GetMerchandiseByEvent(pool, eventID, 4)
    if err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load merchandise preview"})
      return
    }
    c.JSON(http.StatusOK, gin.H{"data": items})
  }
}
 
// ─── Organiser merch management ───────────────────────────────────────────
 
func OrgCreateMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
  return func(c *gin.Context) {
    eventID := c.Param("id")
    var dto models.CreateMerchandiseDTO
    if err := c.ShouldBindJSON(&dto); err != nil {
      c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
      return
    }
    item, err := repository.CreateMerchandise(pool, eventID, &dto)
    if err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create merchandise"})
      return
    }
    c.JSON(http.StatusCreated, item)
  }
}
 
func OrgCreateVariantHandler(pool *pgxpool.Pool) gin.HandlerFunc {
  return func(c *gin.Context) {
    merchID := c.Param("id")
    var dto models.CreateVariantDTO
    if err := c.ShouldBindJSON(&dto); err != nil {
      c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
      return
    }
    variant, err := repository.CreateVariant(pool, merchID, &dto)
    if err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create variant"})
      return
    }
    c.JSON(http.StatusCreated, variant)
  }
}
