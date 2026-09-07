package handlers

import (
	"context"
	"crypto/rsa"
	"net/http"
	"time"

	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── POS attestation & key delivery (Phase 4c) ─────────────────────────────────
//
// The gate that decides whether an event's tag key goes to a physical device
// somebody else is holding.
//
// THE FLOW, and why each step exists:
//
//	1. POST .../pos/attestation/nonce
//	   Server issues a single-use challenge. Without it, a device could replay
//	   attestation evidence captured from a clean handset — the first thing
//	   anyone rooting a till would try.
//
//	2. POST .../pos/attestation/enroll
//	   Device returns a Play Integrity token AND a Keystore attestation chain,
//	   both carrying that challenge. Server verifies both, checks the chain
//	   against the revocation list, and — only if everything passes — wraps the
//	   event tag key to the device's attested public key.
//
// WHAT THIS BUYS, precisely. A cloned band presented to an HONEST till is
// refused, because the till can now verify cryptograms offline. Against a
// COMPROMISED till it buys nothing: that device authors the transaction and can
// report a sale that never happened whether or not it holds a key. The Phase 4a
// caps remain the load-bearing control. Attestation reduces how often a
// compromised device is the one holding a key; it does not make holding one safe.

// AttestationNonceTTL is deliberately short. The nonce is the replay control,
// and a long window is a long window for a replay.
const AttestationNonceTTL = 3 * time.Minute

// IssueAttestationNonceHandler starts an enrolment.
//
//	POST /v1/staff/events/:id/pos/attestation/nonce
//	{ "device_token": "..." }
func IssueAttestationNonceHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			DeviceToken string `json:"device_token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		deviceID, _, err := repository.ResolvePOSDevice(ctx, pool, eventID, in.DeviceToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unrecognised or suspended till"})
			return
		}

		nonce, err := services.NewAttestationNonce()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to issue challenge"})
			return
		}
		if err := repository.IssueAttestationNonce(ctx, pool, deviceID, nonce, AttestationNonceTTL); err != nil {
			log.Error().Err(err).Str("device_id", deviceID).Msg("attestation: nonce issue failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to issue challenge"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"nonce":      nonce,
			"expires_in": int(AttestationNonceTTL.Seconds()),
			// Stated explicitly so the client cannot get it wrong silently: the
			// SAME value must go into the Play Integrity request AND the
			// Keystore attestation challenge. Binding only one leaves the other
			// half replayable.
			"bind_to": []string{"play_integrity_request_hash", "keystore_attestation_challenge"},
		})
	}
}

// EnrollDeviceHandler verifies attestation and releases the event tag key.
//
//	POST /v1/staff/events/:id/pos/attestation/enroll
//	{ "device_token": "...", "nonce": "...",
//	  "integrity_token": "...", "attestation_chain": ["<base64 DER>", ...] }
func EnrollDeviceHandler(
	pool *pgxpool.Pool,
	integrity services.IntegrityVerifier,
	keystore services.KeyAttestationVerifier,
	revocation *services.RevocationChecker,
	tagCrypto *services.TagCryptoService,
	policy services.AttestationPolicy,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			DeviceToken      string   `json:"device_token" binding:"required"`
			Nonce            string   `json:"nonce" binding:"required"`
			IntegrityToken   string   `json:"integrity_token" binding:"required"`
			AttestationChain []string `json:"attestation_chain" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		deviceID, _, err := repository.ResolvePOSDevice(ctx, pool, eventID, in.DeviceToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unrecognised or suspended till"})
			return
		}

		// Offline spend must be enabled for this event before any key moves.
		// Checked first because a disabled event should never even attempt
		// verification — there is nothing to release at the end of it.
		enabled, keyVersion, err := repository.EventOfflineEnabled(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read event policy"})
			return
		}
		if !enabled {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Offline spend is not enabled for this event",
			})
			return
		}

		// Consume the nonce BEFORE doing expensive verification. A replayed
		// nonce should cost one indexed update, not a round trip to Google.
		if err := repository.ConsumeAttestationNonce(ctx, pool, deviceID, in.Nonce); err != nil {
			verdict := string(services.AttestFailedStale)
			_, _ = repository.RecordAttestation(ctx, pool, deviceID, eventID, verdict, "",
				map[string]any{"reason": err.Error()})
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Challenge is unknown, expired, or already used",
			})
			return
		}

		fail := func(verdict services.AttestationVerdict, reason, secLevel string, status int) {
			_, _ = repository.RecordAttestation(ctx, pool, deviceID, eventID,
				string(verdict), secLevel, map[string]any{"reason": reason})
			log.Warn().Str("device_id", deviceID).Str("verdict", string(verdict)).
				Str("reason", reason).Msg("attestation: enrolment refused")
			c.JSON(status, gin.H{"error": "Attestation failed", "reason": reason,
				"verdict": string(verdict)})
		}

		// ── Play Integrity ───────────────────────────────────────────────────
		claims, err := integrity.Verify(ctx, in.IntegrityToken)
		if err != nil {
			fail(services.AttestError, "integrity token could not be verified", "",
				http.StatusBadGateway)
			return
		}

		// ── Keystore chain ───────────────────────────────────────────────────
		der, err := services.DecodeChainB64(in.AttestationChain)
		if err != nil {
			fail(services.AttestFailedChain, "malformed attestation chain", "",
				http.StatusBadRequest)
			return
		}
		hw, err := keystore.Verify(ctx, der)
		if err != nil {
			fail(services.AttestError, "attestation chain could not be verified", "",
				http.StatusBadGateway)
			return
		}

		// ── Revocation ───────────────────────────────────────────────────────
		// The check that catches the most deliberately compromised devices: an
		// extracted attestation key produces a chain that verifies PERFECTLY,
		// because Google really did issue it. Only the published list sees it.
		serials, err := services.ChainSerials(der)
		if err != nil {
			fail(services.AttestFailedChain, "could not read chain serials",
				hw.SecurityLevel, http.StatusBadRequest)
			return
		}
		if ok, reason, err := revocation.CheckChain(ctx, serials, time.Now()); err != nil || !ok {
			if reason == "" {
				reason = "revocation status unavailable"
			}
			fail(services.AttestFailedChain, reason, hw.SecurityLevel, http.StatusForbidden)
			return
		}

		// ── Policy ───────────────────────────────────────────────────────────
		verdict, reason := services.EvaluateAttestation(policy, in.Nonce, claims, hw, time.Now())
		if verdict != services.AttestPassed {
			fail(verdict, reason, hw.SecurityLevel, http.StatusForbidden)
			return
		}

		// The attested key must be RSA. Not our preference — Android's
		// SECURE_KEY_IMPORT requires an RSA wrapping key, and RSA-OAEP is the
		// fallback wrap. A device that generated an EC key gets told exactly
		// that, rather than an opaque failure it cannot act on.
		pub, ok := hw.PublicKey.(*rsa.PublicKey)
		if !ok || pub == nil {
			// Naming what they actually sent turns a dead end into an
			// instruction. An EC key is a legitimate thing for Keystore to
			// generate; it just cannot be used as a wrapping key.
			detail := "attested key must be RSA — regenerate the keystore entry with KEY_ALGORITHM_RSA"
			if hw.KeyAlgorithm == "ec" {
				detail = "attested key is EC; Android's SECURE_KEY_IMPORT requires RSA — " +
					"regenerate the keystore entry with KEY_ALGORITHM_RSA"
			}
			fail(services.AttestFailedHardware, detail, hw.SecurityLevel, http.StatusBadRequest)
			return
		}

		attestationID, err := repository.RecordAttestation(ctx, pool, deviceID, eventID,
			string(services.AttestPassed), hw.SecurityLevel,
			map[string]any{"package": claims.PackageName})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record attestation"})
			return
		}

		// ── Key release ──────────────────────────────────────────────────────
		eventKey, err := tagCrypto.EventKey(eventID, keyVersion)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to derive event key"})
			return
		}

		wrapped, err := services.WrapKeyForDevice(pub, eventKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to wrap key"})
			return
		}

		// Expiry is what makes a stolen till a bounded problem. Scoped to the
		// event's admission window, mirroring how the gate credential is scoped.
		expiresAt, err := repository.EventScanCloses(ctx, pool, eventID)
		if err != nil {
			expiresAt = time.Now().Add(12 * time.Hour)
		}

		if err := repository.MarkDeviceAttested(ctx, pool, deviceID,
			services.EncodePublicKeyB64(pub)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable offline mode"})
			return
		}
		grantID, err := repository.RecordKeyGrant(ctx, pool, deviceID, eventID,
			attestationID, keyVersion, expiresAt)
		if err != nil {
			log.Error().Err(err).Str("device_id", deviceID).Msg("attestation: grant not recorded")
		}

		log.Info().Str("device_id", deviceID).Str("event_id", eventID).
			Str("security_level", hw.SecurityLevel).Int("key_version", keyVersion).
			Msg("attestation: device enrolled for offline spend")

		c.JSON(http.StatusCreated, gin.H{
			"grant_id":       grantID,
			"key_version":    keyVersion,
			"wrapped_key":    wrapped,
			"wrap_algorithm": "RSA-OAEP-SHA256",
			"wrap_label":     "tickethawk-tag-key-v1",
			"expires_at":     expiresAt,
			"security_level": hw.SecurityLevel,
		})
	}
}

// RevokeDeviceOfflineHandler pulls a till back to online-only.
//
//	POST /v1/staff/events/:id/pos/attestation/revoke
//	{ "device_id": "...", "reason": "reported stolen at gate 3" }
//
// Manager-only: this is the lever an organiser pulls when a till goes missing,
// and it kills any live key grant along with offline mode.
func RevokeDeviceOfflineHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			DeviceID string `json:"device_id" binding:"required,uuid"`
			Reason   string `json:"reason" binding:"required,max=200"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.RevokeDeviceOffline(ctx, pool, in.DeviceID, in.Reason); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke device"})
			return
		}

		log.Warn().Str("device_id", in.DeviceID).Str("event_id", eventID).
			Str("reason", in.Reason).Msg("attestation: offline mode revoked")

		c.JSON(http.StatusOK, gin.H{
			"message": "Device returned to online-only. Any live key grant has been revoked.",
		})
	}
}

// ListAttestationsHandler is the organiser's device health view.
//
//	GET /v1/staff/events/:id/pos/attestations
//
// Failures are the point: a till failing repeatedly is a device somebody is
// working on, and that pattern is only visible if refusals are recorded.
func ListAttestationsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}
		rows, err := repository.ListDeviceAttestations(ctx, pool, eventID, 100)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load attestations"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"attestations": rows})
	}
}