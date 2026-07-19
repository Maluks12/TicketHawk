package handlers

import (
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Admin KYC review endpoints, mounted under the existing /v1/admin group.

// AdminListKYCHandler lists submissions by status (default: pending_review).
//
//	GET /v1/admin/kyc?status=pending_review
func AdminListKYCHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := repository.ListKYCSubmissions(pool, c.Request.Context(), c.Query("status"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if rows == nil {
			rows = []repository.AdminKYCQueueRow{}
		}
		c.JSON(http.StatusOK, gin.H{"submissions": rows})
	}
}

// AdminGetKYCDetailHandler returns the full review view, including the
// decrypted SA ID number (so the reviewer can compare against the photo) and
// the computed bank-name match.
//
//	GET /v1/admin/kyc/:id
func AdminGetKYCDetailHandler(pool *pgxpool.Pool, kycService *services.KYCService) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := repository.GetAdminKYCDetail(pool, c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		detail := raw.Detail
		if len(raw.IDEncrypted) > 0 {
			if idNum, dErr := kycService.Decrypt(raw.IDEncrypted); dErr == nil {
				detail.SAIDNumber = &idNum
			} else {
				log.Error().Err(dErr).Str("organizer_id", detail.OrganizerID).
					Msg("kyc admin: stored ID failed to decrypt — was the encryption key rotated?")
			}
		}
		if len(raw.RegistryRaw) > 0 {
			// Pass the stored provider response through as-is for the UI.
			detail.RegistryCheck = gin.H{}
			c.JSON(http.StatusOK, gin.H{"detail": detail, "registry_raw": string(raw.RegistryRaw)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"detail": detail})
	}
}

// AdminGetKYCDocumentHandler streams a stored identity document. This is the
// ONLY way the documents leave the database — authenticated admin access,
// no CDN, no public URL.
//
//	GET /v1/admin/kyc/:id/document           (ID photo)
//	GET /v1/admin/kyc/:id/proof-of-address
func AdminGetKYCDocumentHandler(pool *pgxpool.Pool, kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, mime, err := repository.GetKYCDocument(pool, c.Request.Context(), c.Param("id"), kind)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Data(http.StatusOK, mime, data)
	}
}

// AdminDecideKYCHandler approves or rejects a pending submission.
//
//	POST /v1/admin/kyc/:id/decision  {"approve": true}
//	POST /v1/admin/kyc/:id/decision  {"approve": false, "reason": "..."}
func AdminDecideKYCHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminID, _ := c.Get("user_id")

		var dto models.AdminKYCDecisionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid decision payload"})
			return
		}
		if !dto.Approve && dto.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A rejection reason is required — the organizer sees it"})
			return
		}

		err := repository.AdminDecideKYC(pool, c.Request.Context(), c.Param("id"), adminID.(string), dto.Approve, dto.Reason)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}

		msg := "Identity verified"
		if !dto.Approve {
			msg = "Submission rejected — the organizer can fix and resubmit"
		}
		c.JSON(http.StatusOK, gin.H{"message": msg})
	}
}

// AdminRunRegistryCheckHandler re-runs the DHA registry check for one
// submission (requires DIDIT_API_KEY). Auto-verifies on a full match if the
// submission is still pending.
//
//	POST /v1/admin/kyc/:id/registry-check
func AdminRunRegistryCheckHandler(kycService *services.KYCService) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := kycService.RunRegistryCheck(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"conclusive": res.Conclusive,
			"full_match": res.FullMatch,
			"match_type": res.MatchType,
			"raw":        string(res.Raw),
		})
	}
}