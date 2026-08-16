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

		// Whether THIS caller may open THIS organiser's documents, decided by
		// the same function the document endpoint's gate uses.
		//
		// Sent with the detail so the console renders the right panel on the
		// first paint, rather than optimistically requesting the photo and
		// discovering a 403. Two copies of a rule drift; one function called
		// twice cannot.
		if userID, ok := c.Get("user_id"); ok {
			if uid, _ := userID.(string); uid != "" {
				st, aErr := EvaluateDocumentAccess(c.Request.Context(), pool, uid, c.Param("id"))
				if aErr != nil {
					// Fail closed on the DISPLAY too. A panel that renders as
					// "you may look" when the server would refuse produces a
					// confusing dead button; locked is the honest default.
					log.Warn().Err(aErr).Str("organizer_id", c.Param("id")).
						Msg("kyc admin: document access evaluation failed — rendering as locked")
					detail.DocumentLocked = true
					detail.CanViewDocument = false
					detail.DocumentLockReason = "Access could not be verified. Try again shortly."
				} else {
					detail.DocumentLocked = st.Locked
					detail.CanViewDocument = st.CanView
					detail.DocumentViaGrant = st.ViaGrant
					detail.DocumentGrantExpiresAt = st.GrantExpiresAt
					detail.DocumentLockReason = st.Reason
				}
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

// AdminGetKYCDocumentHandler is GONE. It used to be registered as
//
//	GET /v1/admin/kyc/:id/document
//	GET /v1/admin/kyc/:id/proof-of-address
//
// on the plain admin group, with no capability check and no access log. Because
// RequireStaffWrite only gates mutating verbs, a read_only invitee could fetch
// any organiser's ID photograph with a single GET — and so could anyone else
// holding users.role='admin', at any point in the organiser's lifecycle, long
// after verification, leaving no trace.
//
// That route was the entire two-phase rule's back door: RequireDocumentAccess,
// the master-admin lock, the grant system and kyc_document_access_log all sat
// beside it doing nothing, because the console fetched the bytes from here.
//
// Documents now leave only through POST /admin/kyc/:id/document — see
// identity_document_handler.go. Deleting the handler rather than gating it is
// deliberate: a gated GET is one careless route registration away from being
// ungated again, and there is nothing this handler did that the POST does not.

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

		// KYC-gated role: approval is the moment the applicant becomes a real
		// organiser (users.role flips, organizer row goes approved/verified).
		if dto.Approve {
			if err := repository.PromoteOrganizerAfterKYC(pool, c.Request.Context(), c.Param("id")); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Identity approved but organiser activation failed — re-run the decision or registry check",
				})
				return
			}
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