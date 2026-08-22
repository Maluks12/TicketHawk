package handlers

import (
	"errors"
	"net/http"
	"strings"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ═══════════════════════════════════════════════════════════════════════════
// Admin: event information catalogue
//
// Lets staff add, relabel, reorder and retire information fields without SQL.
//
// ── What is deliberately NOT editable here ──────────────────────────────────
//
// `key` cannot be changed after creation, and `value_type` cannot be changed
// once any event uses the field.
//
// A key rename would orphan every event_info_items row pointing at the old one
// — the foreign key is ON DELETE RESTRICT, so the write would simply fail, but
// only after an admin had filled in a form believing it would work.
//
// A value_type change is worse because it succeeds. Flipping `parking` from
// text to enum leaves hundreds of events holding free prose that no longer
// validates: the organiser cannot save their form until they retype a field
// they never touched, and the error names a field they did not change.
//
// Retiring is done with is_active, never deletion. Deactivating hides a field
// from the picker while every existing event keeps its value — see the read
// path in GetEventInfoHandler, which loads the full catalogue including
// inactive entries for exactly this reason.
// ═══════════════════════════════════════════════════════════════════════════

// AdminListInfoCatalogueHandler lists every field, including retired ones.
//
//	GET /v1/admin/event-info/catalogue
//
// Unlike the organiser-facing endpoint this includes inactive entries — an
// admin managing the catalogue needs to see what they retired in order to
// bring it back.
func AdminListInfoCatalogueHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		entries, err := repository.GetInfoCatalogue(pool, true)
		if err != nil {
			utils.InternalError(c, err, "failed to load information catalogue")
			return
		}

		usage, err := repository.GetInfoCatalogueUsage(pool)
		if err != nil {
			// Usage counts are decoration on a management screen. Losing them
			// should not cost the admin the screen.
			usage = map[string]int{}
		}

		c.JSON(http.StatusOK, gin.H{
			"catalogue": entries,
			// How many events use each field. This is what tells an admin
			// whether retiring something is harmless or will change 400 pages.
			"usage": usage,
		})
	}
}

// AdminCreateInfoFieldHandler adds a catalogue field.
//
//	POST /v1/admin/event-info/catalogue
func AdminCreateInfoFieldHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.AdminCreateInfoFieldDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		entry, err := models.ValidateNewInfoField(&dto)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.CreateInfoCatalogueEntry(pool, entry); err != nil {
			if errors.Is(err, repository.ErrInfoKeyExists) {
				c.JSON(http.StatusConflict, gin.H{
					"error": "a field with that key already exists",
					"code":  "key_exists",
				})
				return
			}
			utils.InternalError(c, err, "failed to create field")
			return
		}

		c.JSON(http.StatusCreated, gin.H{"field": entry})
	}
}

// AdminUpdateInfoFieldHandler edits a catalogue field's presentation.
//
//	PATCH /v1/admin/event-info/catalogue/:key
//
// Label, hint, display order, enum options and the review flag. Not the key,
// and not the value type once the field is in use — see the file header.
func AdminUpdateInfoFieldHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.ToLower(strings.TrimSpace(c.Param("key")))

		var dto models.AdminUpdateInfoFieldDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		existing, err := repository.GetInfoCatalogueEntry(pool, key)
		if err != nil {
			if errors.Is(err, repository.ErrInfoKeyNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "field not found"})
				return
			}
			utils.InternalError(c, err, "failed to load field")
			return
		}

		// Narrowing an enum is the dangerous edit. Dropping "16+" from the
		// options while events hold that value leaves those organisers unable
		// to save their own form — the validator rejects a value they never
		// chose to enter. Removing an option in use is refused; adding one is
		// always safe.
		if dto.EnumOptions != nil {
			inUse, err := repository.GetInfoValuesInUse(pool, key)
			if err == nil {
				if orphaned := models.OptionsRemovedFromUse(*dto.EnumOptions, inUse); len(orphaned) > 0 {
					c.JSON(http.StatusConflict, gin.H{
						"error": "some events still use options you are removing: " +
							strings.Join(orphaned, ", "),
						"code":     "options_in_use",
						"orphaned": orphaned,
					})
					return
				}
			}
		}

		updated, err := models.ValidateInfoFieldUpdate(existing, &dto)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.UpdateInfoCatalogueEntry(pool, updated); err != nil {
			utils.InternalError(c, err, "failed to update field")
			return
		}

		c.JSON(http.StatusOK, gin.H{"field": updated})
	}
}

// AdminSetInfoFieldActiveHandler retires or restores a field.
//
//	PUT /v1/admin/event-info/catalogue/:key/active
//
// Retiring hides the field from the organiser picker. Every event that already
// uses it keeps its value and keeps rendering it — the alternative, deleting
// the row, is refused by the ON DELETE RESTRICT foreign key precisely so that
// information organisers entered cannot vanish because someone tidied a list.
func AdminSetInfoFieldActiveHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.ToLower(strings.TrimSpace(c.Param("key")))

		var body struct {
			IsActive *bool `json:"is_active" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		if err := repository.SetInfoCatalogueActive(pool, key, *body.IsActive); err != nil {
			if errors.Is(err, repository.ErrInfoKeyNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "field not found"})
				return
			}
			utils.InternalError(c, err, "failed to update field")
			return
		}

		used, _ := repository.GetInfoFieldUsageCount(pool, key)

		c.JSON(http.StatusOK, gin.H{
			"key":       key,
			"is_active": *body.IsActive,
			// So the console can say "hidden from new events; 42 existing
			// events keep showing it" rather than leaving an admin to guess
			// whether they just deleted something.
			"events_using": used,
		})
	}
}

// AdminReorderInfoCatalogueHandler sets display order in bulk.
//
//	PUT /v1/admin/event-info/catalogue/order
//
// One call for the whole list rather than a PATCH per field: the console
// reorders by drag, which moves several fields at once, and applying that as N
// separate requests would leave the list visibly half-sorted if one failed.
func AdminReorderInfoCatalogueHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Keys []string `json:"keys" binding:"required,min=1,max=100"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		if err := repository.ReorderInfoCatalogue(pool, body.Keys); err != nil {
			utils.InternalError(c, err, "failed to reorder catalogue")
			return
		}

		entries, err := repository.GetInfoCatalogue(pool, true)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"reordered": true})
			return
		}
		c.JSON(http.StatusOK, gin.H{"catalogue": entries})
	}
}