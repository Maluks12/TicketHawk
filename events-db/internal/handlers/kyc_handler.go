package handlers

import (
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
		c.JSON(http.StatusOK, gin.H{
			"kyc": profile,
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide your legal first name, legal last name, and 13-digit SA ID number"})
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