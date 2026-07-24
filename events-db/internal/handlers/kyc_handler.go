package handlers

import (
	"strings"
	"errors"
	"io"
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Organizer-facing KYC endpoints, mounted under the authenticated
// /v1/organiser group. Reuses resolveOrganizerID from ad_account_handler.go.

// GetKYCStatusHandler returns the organizer's KYC state plus the requirements
// matrix so clients can render "what you still need" per event type.
//
//	GET /v1/organiser/kyc
func GetKYCStatusHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}
		profile, err := repository.GetKYCProfile(pool, c.Request.Context(), organizerID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load verification status"})
			return
		}
		tier, _ := repository.GetKYCLevel(pool, c.Request.Context(), organizerID)
		c.JSON(http.StatusOK, gin.H{
			"kyc":      profile,
			"kyc_level": tier,
			// What each tier unlocks — the create-event UI renders this.
			"tiers": gin.H{
				"basic": gin.H{"tier": 1, "unlocks": "Private free events"},
				"full":  gin.H{"tier": 2, "unlocks": "Paid events and public events"},
			},
			"requirements": gin.H{
				"free_private": models.RequirementsFor(models.VisibilityPrivateLink, false),
				"free_public":  models.RequirementsFor(models.VisibilityPublic, false),
				"paid_private": models.RequirementsFor(models.VisibilityPrivateLink, true),
				"paid_public":  models.RequirementsFor(models.VisibilityPublic, true),
			},
		})
	}
}

// SubmitKYCHandler accepts the multipart KYC submission:
//
//	POST /v1/organiser/kyc
//	  fields: legal_first_name, legal_last_name, sa_id_number
//	  files : id_document (required, JPEG/PNG/WebP ≤5MB)
//	          proof_of_address (optional, +PDF allowed)
func SubmitKYCHandler(pool *pgxpool.Pool, kycService *services.KYCService) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, ok := resolveOrganizerID(c, pool)
		if !ok {
			return
		}

		var dto models.SubmitKYCDTO
		if err := c.ShouldBind(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide your legal first name and legal last name as they appear on your ID (and, if you have no ID number on file, your 13-digit SA ID number)"})
			return
		}

		idDoc, idMime, err := readKYCUpload(c, "id_document", false)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ID document: " + err.Error()})
			return
		}
		if idDoc == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A photo of your SA ID is required (JPEG, PNG or WebP, up to 5MB)"})
			return
		}

		proof, proofMime, err := readKYCUpload(c, "proof_of_address", true)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Proof of address: " + err.Error()})
			return
		}

		status, err := kycService.Submit(c.Request.Context(), services.KYCSubmission{
			OrganizerID:    organizerID,
			LegalFirstName: dto.LegalFirstName,
			LegalLastName:  dto.LegalLastName,
			SAIDNumber:     dto.SAIDNumber,
			IDDocument:     idDoc,
			IDDocumentMime: idMime,
			ProofOfAddress: proof,
			ProofMime:      proofMime,
		})
		if err != nil {
			if errors.Is(err, repository.ErrKYCNotSubmittable) {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			// Validation errors (checksum, age, file type) are user-facing.
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if status == models.KYCVerified {
			c.JSON(http.StatusOK, gin.H{
				"message": "Identity verified — your organiser account is now active. Sign in again to open your dashboard.",
				"status":  status,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "Identity submitted for review. Your organiser account will be activated once it's approved (usually within 1 business day).",
			"status":  status,
		})
	}
}

// readKYCUpload pulls one multipart file, enforces the size cap, and sniffs
// the real type. Returns (nil, "", nil) when the field is absent.
func readKYCUpload(c *gin.Context, field string, allowPDF bool) ([]byte, string, error) {
	fh, err := c.FormFile(field)
	if err != nil {
		return nil, "", nil // absent — caller decides if that's OK
	}
	if fh.Size > models.KYCDocMaxBytes {
		return nil, "", errors.New("file is larger than 5MB")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, "", errors.New("could not read the uploaded file")
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, models.KYCDocMaxBytes+1))
	if err != nil {
		return nil, "", errors.New("could not read the uploaded file")
	}
	if len(data) > models.KYCDocMaxBytes {
		return nil, "", errors.New("file is larger than 5MB")
	}

	mime, err := services.SniffKYCDocument(data, allowPDF)
	if err != nil {
		log.Debug().Str("field", field).Err(err).Msg("kyc: rejected upload")
		return nil, "", err
	}
	return data, mime, nil
}

// SubmitBasicKYCHandler performs the TIER-1 onboarding step in one call:
// validates name + SA ID locally, creates the organizer profile if the user
// doesn't have one yet, stores the identity, and grants the organiser role.
// Auth-only (NOT organiser-only) — this IS how a plain account becomes an
// organiser able to create private free events.
//
//	POST /v1/organiser/kyc/basic
//	Body: { "legal_first_name", "legal_last_name", "sa_id_number" }
func SubmitBasicKYCHandler(pool *pgxpool.Pool, kycService *services.KYCService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")

		var dto struct {
			LegalFirstName string `json:"legal_first_name" binding:"required,min=2,max=100"`
			LegalLastName  string `json:"legal_last_name" binding:"required,min=2,max=100"`
			SAIDNumber     string `json:"sa_id_number" binding:"required,len=13"`
		}
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide your legal first name, last name and 13-digit SA ID number"})
			return
		}

		ctx := c.Request.Context()

		// Get-or-create the organizer profile (mirrors register-organizer's
		// derivation, but the business name comes from the LEGAL name the
		// person just gave us — better default than an email prefix).
		org, err := repository.GetOrganizerByUserID(pool, userID.(string))
		if err != nil {
			var email string
			_ = pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID.(string)).Scan(&email)
			org, err = repository.CreateOrganizer(pool, &models.CreateOrganizerDTO{
				BusinessName:  strings.TrimSpace(dto.LegalFirstName + " " + dto.LegalLastName),
				BusinessEmail: email,
				BusinessPhone: "N/A",
				Description:   "Organiser profile created during identity onboarding. This description can be edited later from the organiser dashboard settings.",
			}, userID.(string))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create your organiser profile"})
				return
			}
		}

		if err := kycService.SubmitBasic(ctx, org.ID,
			strings.TrimSpace(dto.LegalFirstName), strings.TrimSpace(dto.LegalLastName), strings.TrimSpace(dto.SAIDNumber)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.PromoteOrganizerBasic(pool, ctx, org.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Identity saved but activation failed — try again"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Profile complete — you can now create private free events. Complete full verification to unlock paid and public events.",
			"kyc_level": models.KYCLevelBasic,
			// The client's current access token still carries the old role;
			// it must refresh the session to pick up 'organizer'.
			"session_refresh_required": true,
		})
	}
}