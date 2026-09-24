package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"events-db/internal/accountguard"
	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Account restrictions — MASTER ADMIN ONLY ─────────────────────────────────
//
// Disable an account for a number of days, ban it, or block single features
// (migration 000094). These handlers write restrictions and reload the
// registry. Enforcement is elsewhere: internal/accountguard with
// AuthMiddleware and RequireAccountFeature on every request, and the payout
// queries for money.

// RequireMasterAdminForRestrictions gates the restriction routes on
// users.is_master_admin — no console role reaches them. Same shape as
// RequireMasterAdminForFees: fails closed, and a refusal is written to
// staff_audit_log as a denied attempt.
func RequireMasterAdminForRestrictions(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var isMaster bool
		var email string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(is_master_admin, FALSE), email FROM users WHERE id = $1`,
			userID,
		).Scan(&isMaster, &email)
		if err != nil {
			log.Error().Err(err).Msg("restrictions: master admin check failed")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}

		if !isMaster {
			// Captured on the request goroutine — gin recycles the context
			// once the chain returns (see auditDenied in middleware).
			path := c.Request.URL.Path
			ip := c.ClientIP()
			ua := c.Request.UserAgent()
			go func() {
				wctx, wcancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer wcancel()
				_, werr := pool.Exec(wctx, `
					INSERT INTO staff_audit_log
						(actor_id, actor_email, actor_role, action, allowed, detail, ip, user_agent)
					VALUES ($1, $2, (SELECT staff_role FROM users WHERE id = $1),
					        'user.restrict', FALSE,
					        jsonb_build_object('reason', 'not master admin', 'path', $3::text),
					        NULLIF($4, '')::inet, $5)
				`, userID, email, path, ip, ua)
				if werr != nil {
					log.Warn().Err(werr).Msg("restrictions: denied-audit write failed")
				}
			}()
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "account restrictions require master admin access",
			})
			return
		}

		c.Set("actor_email", email)
		c.Next()
	}
}

// restrictionLimits is echoed to the console so its limits and feature list
// come from the server rather than a second copy of the constants.
func restrictionLimits() gin.H {
	features := make([]gin.H, 0, len(models.RestrictionFeatures))
	for _, f := range models.RestrictionFeatures {
		features = append(features, gin.H{"key": f, "label": f.Label()})
	}
	return gin.H{
		"max_days":   models.MaxRestrictionDays,
		"min_reason": models.MinRestrictionReasonLen,
		"max_reason": models.MaxRestrictionReasonLen,
		"features":   features,
	}
}

func restrictionActor(c *gin.Context) repository.AuditActor {
	return repository.AuditActor{
		UserID:    c.GetString("user_id"),
		Email:     c.GetString("actor_email"),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
}

// restrictionDetail is the console's detail panel: the account, its history,
// and whether the caller may act on it. The server decides can_restrict so the
// screen never offers an action the API will refuse.
func restrictionDetail(acc *repository.RestrictableAccount, history []repository.RestrictionRecord, actorID string) gin.H {
	canRestrict, why := true, ""
	switch {
	case acc.UserID == actorID:
		canRestrict, why = false, "You can't restrict your own account."
	case acc.IsMasterAdmin:
		canRestrict, why = false, "Master admin accounts can't be restricted."
	}
	return gin.H{
		"account":       acc,
		"history":       history,
		"can_restrict":  canRestrict,
		"cannot_reason": why,
		"limits":        restrictionLimits(),
	}
}

// reloadRestrictions pushes a committed change into the in-memory registry now
// rather than at the next refresh. A failure is logged, not returned: the
// change is committed, the refresh picks it up within 30 seconds, and a
// lockout's sessions were already revoked in the same transaction.
func reloadRestrictions() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := accountguard.Reload(ctx); err != nil {
		log.Error().Err(err).Msg("restrictions: saved, but the registry reload failed — it applies at the next refresh")
	}
}

func invalidID(c *gin.Context, id, what string) bool {
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + what + " id"})
		return true
	}
	return false
}

// AdminListRestrictableAccountsHandler backs the picker: restricted accounts
// when q is empty, a search across all accounts otherwise.
//
//	GET /v1/admin/restrictions/users?q=
func AdminListRestrictableAccountsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := strings.TrimSpace(c.Query("q"))
		if r := []rune(q); len(r) > 100 {
			q = string(r[:100])
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		accounts, err := repository.SearchRestrictableAccounts(ctx, pool, q, 50)
		if err != nil {
			log.Error().Err(err).Msg("restrictions: list accounts failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load accounts"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"accounts": accounts,
			"query":    q,
			"limits":   restrictionLimits(),
		})
	}
}

// AdminGetAccountRestrictionsHandler returns one account's restrictions in
// force and its history.
//
//	GET /v1/admin/restrictions/users/:id
func AdminGetAccountRestrictionsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")
		if invalidID(c, userID, "account") {
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		acc, history, err := repository.GetRestrictableAccount(ctx, pool, userID)
		if err != nil {
			if errors.Is(err, repository.ErrRestrictionAccountNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
				return
			}
			log.Error().Err(err).Str("user_id", userID).Msg("restrictions: load account failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load this account"})
			return
		}
		c.JSON(http.StatusOK, restrictionDetail(acc, history, c.GetString("user_id")))
	}
}

type applyRestrictionDTO struct {
	Action   string   `json:"action"`
	Days     *int     `json:"days"`
	Features []string `json:"features"`
	Reason   string   `json:"reason"`
}

// AdminApplyAccountRestrictionHandler disables, bans or blocks features on an
// account.
//
//	POST /v1/admin/restrictions/users/:id
//	{ "action": "disable", "days": 7, "reason": "..." }
//	{ "action": "ban", "reason": "..." }
//	{ "action": "block", "features": ["buy_tickets", "payouts"], "days": null, "reason": "..." }
func AdminApplyAccountRestrictionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")
		if invalidID(c, userID, "account") {
			return
		}

		var dto applyRestrictionDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
		features := make([]models.RestrictionKind, 0, len(dto.Features))
		for _, f := range dto.Features {
			features = append(features, models.RestrictionKind(f))
		}

		actor := restrictionActor(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		res, err := repository.ApplyAccountRestriction(ctx, pool, userID, repository.RestrictionInput{
			Action:   dto.Action,
			Days:     dto.Days,
			Features: features,
			Reason:   dto.Reason,
		}, actor)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrRestrictionAccountNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
			case errors.Is(err, repository.ErrRestrictionSelf),
				errors.Is(err, repository.ErrRestrictionMasterAdmin):
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			case errors.Is(err, repository.ErrRestrictionAction),
				errors.Is(err, repository.ErrRestrictionDays),
				errors.Is(err, repository.ErrRestrictionNoFeatures),
				errors.Is(err, repository.ErrRestrictionFeature),
				errors.Is(err, repository.ErrRestrictionReason):
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			default:
				log.Error().Err(err).Str("user_id", userID).Msg("restrictions: apply failed")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not apply the restriction"})
			}
			return
		}

		reloadRestrictions()

		log.Info().
			Str("user_id", userID).
			Str("actor", actor.Email).
			Str("action", dto.Action).
			Strs("restriction_ids", res.RestrictionIDs).
			Int64("sessions_ended", res.SessionsEnded).
			Msg("admin: account restricted")

		message := restrictionMessage(dto.Action, dto.Days)
		acc, history, err := repository.GetRestrictableAccount(ctx, pool, userID)
		if err != nil {
			// Written and in force; only the refreshed view failed.
			c.JSON(http.StatusOK, gin.H{"message": message, "result": res})
			return
		}
		body := restrictionDetail(acc, history, actor.UserID)
		body["message"] = message
		body["result"] = res
		c.JSON(http.StatusOK, body)
	}
}

func restrictionMessage(action string, days *int) string {
	switch action {
	case repository.RestrictActionDisable:
		n := 0
		if days != nil {
			n = *days
		}
		unit := "days"
		if n == 1 {
			unit = "day"
		}
		return fmt.Sprintf("Account disabled for %d %s and signed out everywhere.", n, unit)
	case repository.RestrictActionBan:
		return "Account banned and signed out everywhere."
	default:
		return "Features blocked."
	}
}

type liftRestrictionDTO struct {
	Note string `json:"note"`
}

// AdminLiftAccountRestrictionHandler ends one restriction in force, now.
//
//	POST /v1/admin/restrictions/lift/:id
//	{ "note": "optional" }
func AdminLiftAccountRestrictionHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		restrictionID := c.Param("id")
		if invalidID(c, restrictionID, "restriction") {
			return
		}

		// The note is optional, so an empty body is fine; a malformed one isn't.
		var dto liftRestrictionDTO
		if err := c.ShouldBindJSON(&dto); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		actor := restrictionActor(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		userID, err := repository.LiftAccountRestriction(ctx, pool, restrictionID, dto.Note, actor)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrRestrictionNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "Restriction not found"})
			case errors.Is(err, repository.ErrRestrictionNotInForce):
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			case errors.Is(err, repository.ErrRestrictionNoteTooLong):
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			default:
				log.Error().Err(err).Str("restriction_id", restrictionID).Msg("restrictions: lift failed")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not lift the restriction"})
			}
			return
		}

		reloadRestrictions()

		log.Info().
			Str("user_id", userID).
			Str("restriction_id", restrictionID).
			Str("actor", actor.Email).
			Msg("admin: account restriction lifted")

		acc, history, err := repository.GetRestrictableAccount(ctx, pool, userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "Restriction lifted."})
			return
		}
		body := restrictionDetail(acc, history, actor.UserID)
		body["message"] = "Restriction lifted."
		c.JSON(http.StatusOK, body)
	}
}