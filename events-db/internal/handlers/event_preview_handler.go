package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ═══════════════════════════════════════════════════════════════════════════
// Dashboard event preview
//
// Lets an organiser see exactly what their event page will look like BEFORE it
// is published — including drafts, which the public endpoint refuses.
//
// ── Why a signed token and not "just render it in the dashboard" ────────────
//
// The obvious alternative is to rebuild the event page as React components
// inside events-organizer. That is faster to ship and wrong within a month: the
// replica drifts from the real page the moment either side changes, and a
// preview that lies is worse than no preview, because the organiser makes
// decisions on it.
//
// So the preview renders the REAL page — web-mobile or web-desktop, in a device
// frame — pointed at the real event URL with `?preview=<token>` appended. One
// source of truth, and any change to the page shows up in the preview for free.
//
// ── Why the token is scoped this tightly ────────────────────────────────────
//
// The token makes a NON-PUBLIC event readable by an unauthenticated browser.
// That is a real widening of access, so it is fenced in four ways:
//
//	scope       "event_preview" and nothing else, so an access token cannot be
//	            used here and this cannot be used as an access token
//	event_id    one event. A token for event A cannot read event B.
//	10 minutes  long enough to look at the page, short enough that a token
//	            leaked in a screenshot or a shared URL is dead on arrival
//	issuer      the organiser's user id, recorded in the claims so a leak is
//	            traceable to the account it was minted for
//
// It is deliberately NOT a database row. A revocation table would need its own
// cleanup and its own failure mode; a 10-minute expiry achieves the same thing
// with nothing to maintain.
// ═══════════════════════════════════════════════════════════════════════════

const (
	previewTokenTTL   = 10 * time.Minute
	previewTokenScope = "event_preview"
)

// Two distinct errors, because they mean different things to the person
// reading the message.
//
// ErrPreviewTokenExpired is the ORDINARY case: the organiser left the preview
// tab open past ten minutes. "Generate a new one" is exactly the right advice.
//
// ErrInvalidPreviewToken is everything else — a bad signature, the wrong
// scope, a malformed token. Telling that user to "generate a new one" sends
// them to fix something that was never broken, and during an incident it means
// somebody probing the endpoint gets told "expired" while support fields a
// report about a link that never worked.
var (
	ErrPreviewTokenExpired = errors.New("preview token expired")
	ErrInvalidPreviewToken = errors.New("invalid preview token")
)

// MintEventPreviewToken issues a preview token for one event.
func MintEventPreviewToken(cfg *config.Config, eventID, userID string) (string, time.Time, error) {
	expires := time.Now().Add(previewTokenTTL)

	claims := jwt.MapClaims{
		"scope":    previewTokenScope,
		"event_id": eventID,
		"user_id":  userID,
		"exp":      expires.Unix(),
		"iat":      time.Now().Unix(),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("mint preview token: %w", err)
	}
	return signed, expires, nil
}

// ParseEventPreviewToken validates a token and returns the event id it grants
// access to, plus the user it was minted for.
//
// The algorithm is pinned to HS256. Without that check a caller could present a
// token with alg "none", which some JWT libraries have historically accepted as
// valid with no signature at all — turning the preview into an open door to
// every unpublished event.
func ParseEventPreviewToken(cfg *config.Config, raw string) (eventID, userID string, err error) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		// jwt.Parse wraps a genuine expiry as ErrTokenExpired. Everything else
		// — bad signature, wrong algorithm, malformed — is not an expiry and
		// must not be reported as one.
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", "", ErrPreviewTokenExpired
		}
		return "", "", ErrInvalidPreviewToken
	}
	if !token.Valid {
		return "", "", ErrInvalidPreviewToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", ErrInvalidPreviewToken
	}

	// An access token is signed with the same secret and would otherwise
	// validate here. The scope check is what keeps the two apart.
	if scope, _ := claims["scope"].(string); scope != previewTokenScope {
		return "", "", ErrInvalidPreviewToken
	}

	eventID, _ = claims["event_id"].(string)
	userID, _ = claims["user_id"].(string)
	if eventID == "" || userID == "" {
		return "", "", ErrInvalidPreviewToken
	}
	return eventID, userID, nil
}

// ── Endpoints ────────────────────────────────────────────────────────────────

// CreateEventPreviewHandler mints a preview link for an event the caller owns.
//
//	POST /v1/organiser/events/:id/preview
//
// Returns the token plus ready-made URLs for both web layouts, so the dashboard
// does not have to know how to build them — which would mean the base URL
// living in two places and going stale in one of them.
func CreateEventPreviewHandler(pool *pgxpool.Pool, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		eventID := c.Param("id")

		// Ownership first. Without this any organiser could mint a preview
		// token for anyone else's unpublished event just by knowing its id.
		editCtx, err := repository.GetEventEditContext(pool, eventID, userID)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotOwned) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
				return
			}
			utils.InternalError(c, err, "failed to load event")
			return
		}

		token, expires, err := MintEventPreviewToken(cfg, editCtx.EventID, userID)
		if err != nil {
			utils.InternalError(c, err, "failed to create preview link")
			return
		}

		base := ""
		if len(cfg.FrontendURLs) > 0 {
			base = cfg.FrontendURLs[0]
		}

		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"expires_at": expires,
			"api_url":    fmt.Sprintf("/v1/events/%s/preview?token=%s", eventID, token),
			"web_url":    fmt.Sprintf("%s/events/%s?preview=%s", base, eventID, token),
			// The dashboard frames this in a phone-width iframe. Same page as
			// web_url; the two are separate keys so a future mobile-specific
			// host does not require a dashboard change.
			"web_mobile_url": fmt.Sprintf("%s/events/%s?preview=%s", base, eventID, token),
			"expires_in":     int(previewTokenTTL.Seconds()),
		})
	}
}

// GetEventPreviewHandler serves an event by preview token.
//
//	GET /v1/events/:id/preview?token=...
//
// UNAUTHENTICATED. The token is the whole authorisation, which is why it is
// scoped, event-bound and short-lived.
//
// ── How a draft is made visible ─────────────────────────────────────────────
//
// NOT with GetEventByIDInternal, despite that being the obvious "give me any
// status" function. Its doc comment says NEVER call it from a request handler,
// and it means it: it drops the status predicate entirely, so a bug anywhere in
// the token handling above would expose every organiser's drafts.
//
// Instead the token carries the OWNER'S user id, and that is passed as
// requestingUserID to the ordinary GetEventByID. Its existing predicate is
//
//	AND (e.status = ANY(live) OR e.organizer_id = $3)
//
// so the draft is visible through the same ownership branch the organiser's own
// authenticated requests use. No bypass, no second code path, and if event
// ownership rules change this follows them automatically.
//
// ── The check that matters ──────────────────────────────────────────────────
//
// The event id in the URL is IGNORED for authorisation. Only the id inside the
// signed token decides what is returned. Trusting the URL would let anyone
// holding one valid preview token read every unpublished event on the platform
// by swapping the path segment.
func GetEventPreviewHandler(pool *pgxpool.Pool, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.Query("token")
		if raw == "" {
			// Also accept the header form, so a fetch from the preview iframe
			// can avoid putting the token in a URL that lands in browser
			// history.
			raw = c.GetHeader("X-Preview-Token")
		}
		if raw == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "preview token required"})
			return
		}

		tokenEventID, tokenUserID, err := ParseEventPreviewToken(cfg, raw)
		if err != nil {
			if errors.Is(err, ErrPreviewTokenExpired) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "this preview link has expired — generate a new one from the dashboard",
					"code":  "preview_expired",
				})
				return
			}
			// Deliberately vague about WHY. A caller presenting a forged token
			// learns nothing about which check rejected it, and the dashboard
			// has no branch to write against a detail it cannot act on.
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "this preview link is not valid",
				"code":  "preview_invalid",
			})
			return
		}

		// The URL id is checked against the token only to give a clearer error;
		// the token's id is what is actually loaded either way.
		if urlID := c.Param("id"); urlID != "" && urlID != tokenEventID {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "this preview link is for a different event",
			})
			return
		}

		event, err := repository.GetEventByID(pool, tokenEventID, &tokenUserID)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
				return
			}
			utils.InternalError(c, err, "failed to load preview")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"event": event,
			// Tells the page to render a "PREVIEW — not visible to the public"
			// banner and disable the purchase button. A preview where Get
			// Tickets appears to work invites an organiser to test a purchase
			// against an event that cannot sell.
			"preview": gin.H{
				"is_preview": true,
				"status":     event.Status,
				"is_live":    event.Status == models.EventPublished,
			},
		})
	}
}