package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// expoTokenPattern matches "ExponentPushToken[...]" and the newer
// "ExpoPushToken[...]" form.
//
// Validating shape here rather than at send time keeps junk out of the table
// entirely. Without it, a client bug (registering `undefined`, or an FCM token
// straight from the firebase SDK) silently fills push_tokens with rows that can
// never receive anything, and every future send wastes a slot in a 100-message
// batch on them.
var expoTokenPattern = regexp.MustCompile(`^Expo(nent)?PushToken\[[A-Za-z0-9_\-]+\]$`)

type registerPushTokenRequest struct {
	Token      string `json:"token"      binding:"required"`
	Platform   string `json:"platform"   binding:"required,oneof=ios android"`
	App        string `json:"app"        binding:"omitempty,oneof=consumer studio"`
	DeviceName string `json:"device_name"`
}

// RegisterPushTokenHandler — POST /v1/notifications/push-token
//
// Idempotent by design: the app calls this on every cold start (after the auth
// session settles) as well as on login and on the notifications toggle. Expo
// rotates tokens without warning — on reinstall, on some OS updates, and when
// FCM decides to — so re-registering on launch is the only reliable way to keep
// the registry fresh. An unchanged token just refreshes updated_at.
func RegisterPushTokenHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		var req registerPushTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		token := strings.TrimSpace(req.Token)
		if !expoTokenPattern.MatchString(token) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid push token format — expected ExponentPushToken[...]",
			})
			return
		}

		app := models.PushApp(req.App)
		if app == "" {
			app = models.PushAppConsumer
		}

		// device_name is support metadata, not something to trust or display
		// back to other users. Cap it so a malformed client can't write a
		// megabyte of text per device.
		deviceName := req.DeviceName
		if len(deviceName) > 120 {
			deviceName = deviceName[:120]
		}

		if err := repository.UpsertPushToken(
			pool, c.Request.Context(),
			userID, token,
			models.PushPlatform(req.Platform),
			app,
			deviceName,
		); err != nil {
			utils.InternalError(c, err, "register push token")
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "push token registered"})
	}
}

type unregisterPushTokenRequest struct {
	Token string `json:"token"`
	All   bool   `json:"all"` // drop every device for this account
}

// UnregisterPushTokenHandler — DELETE /v1/notifications/push-token
//
// Called on logout and when the user turns notifications off in Settings.
//
// Logout is the important one: without it, the next person to sign in on that
// handset keeps receiving the previous user's booking confirmations until the
// token happens to rotate. (Registration re-points a token to the new owner, so
// this only matters when nobody signs in afterwards — but "nobody signs in
// afterwards" is exactly the phone-sold-secondhand case.)
//
// Returns 200 whether or not a row matched. The app fires this unconditionally
// on logout, including for users who never granted notification permission, and
// a 404 there would be noise in the client logs, not information.
func UnregisterPushTokenHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		var req unregisterPushTokenRequest
		// Body is optional — ShouldBindJSON's error is deliberately ignored so
		// a bodyless DELETE degrades to "nothing to do" rather than a 400.
		_ = c.ShouldBindJSON(&req)

		if req.All {
			if err := repository.DeleteAllPushTokensForUser(pool, c.Request.Context(), userID); err != nil {
				utils.InternalError(c, err, "unregister all push tokens")
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "all push tokens removed"})
			return
		}

		token := strings.TrimSpace(req.Token)
		if token == "" {
			c.JSON(http.StatusOK, gin.H{"message": "no token supplied — nothing to remove"})
			return
		}

		if err := repository.DeletePushToken(pool, c.Request.Context(), userID, token); err != nil {
			utils.InternalError(c, err, "unregister push token")
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "push token removed"})
	}
}

// SendTestPushHandler — POST /v1/notifications/push-test
//
// Sends a real notification to the CALLER'S OWN devices, through the real
// pipeline: it writes a notifications row exactly like any worker would, which
// trips the dispatcher hook, which enqueues a push. That end-to-end path is the
// point — a handler that posted to Expo directly would happily pass while the
// dispatcher wiring was broken.
//
// It is authed and rate-limited with the rest of the notifications group, and a
// user can only ever target themselves, so it is safe to leave enabled in
// production. It is genuinely useful there: "does push work on YOUR handset"
// is otherwise unanswerable without waiting for a real booking.
func SendTestPushHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		tokens, err := repository.GetPushTargets(pool, c.Request.Context(), []string{userID})
		if err != nil {
			utils.InternalError(c, err, "test push: token lookup")
			return
		}
		if len(tokens) == 0 {
			c.JSON(http.StatusPreconditionFailed, gin.H{
				"error": "no registered devices — enable notifications in the app first",
			})
			return
		}

		if err := repository.CreateNotification(
			pool, c.Request.Context(),
			userID,
			models.NotifBookingConfirmed,
			"🦅 TicketHawk test",
			"Push notifications are working on this device.",
			map[string]any{"test": true},
		); err != nil {
			utils.InternalError(c, err, "test push: create notification")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "test notification queued",
			"devices": len(tokens),
		})
	}
}