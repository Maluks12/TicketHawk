package middleware

import (
	"net/http"

	"events-db/internal/accountguard"
	"events-db/internal/models"

	"github.com/gin-gonic/gin"
)

// RequireAccountFeature refuses a signed-in caller whose account a master
// admin has blocked from this feature (migration 000094). The 403's `error` is
// the popup text: "Your account has been limited. You can't buy tickets."
//
// Register AFTER AuthMiddleware or OptionalAuth — it reads the user_id they
// set. A request with no user_id is a guest and passes: guest checkout checks
// the email it is buying with instead, inside the handler, because only the
// handler has read the body.
//
// A locked-out account (disabled or banned) never reaches this point —
// AuthMiddleware and OptionalAuth have already refused it — so this answers the
// feature question only.
func RequireAccountFeature(feature models.RestrictionKind) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			c.Next()
			return
		}
		if block, blocked := accountguard.FeatureBlock(userID, feature); blocked {
			c.AbortWithStatusJSON(http.StatusForbidden, accountguard.FeatureBlockedBody(userID, block))
			return
		}
		c.Next()
	}
}