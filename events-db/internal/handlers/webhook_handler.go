package handlers
 
import (
    "encoding/json"
    "events-db/internal/services"
    "io"
    "log"
    "net/http"
 
    "github.com/gin-gonic/gin"
    svix "github.com/svix/svix-webhooks/go"
    // go get github.com/svix/svix-webhooks/go
)
 
func StitchWebhookHandler(
    paymentService *services.PaymentService,
    webhookSecret string,
) gin.HandlerFunc {
 
    // Initialise verifier once at startup — not per request
    wh, err := svix.NewWebhook(webhookSecret)
    if err != nil {
        log.Printf("ERROR: invalid webhook secret — check STITCH_WEBHOOK_SECRET is in 'whsec_<base64>' format: %v", err)
        return func(c *gin.Context) {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook handler misconfigured"})
        }
    }
 
    return func(c *gin.Context) {
 
        // Read raw body
        rawBody, err := io.ReadAll(c.Request.Body)
        if err != nil {
            c.JSON(http.StatusBadRequest,
                gin.H{"error": "Failed to read request body"})
            return
        }
 
        // Verify signature + timestamp window in one call
        // ✓ HMAC-SHA256 check
        // ✓ Replay attack prevention (5-min window)
        // ✓ Constant-time comparison
        if err := wh.Verify(rawBody, c.Request.Header); err != nil {
            log.Printf("stitch webhook: invalid signature from %s",
                c.ClientIP())
            c.JSON(http.StatusUnauthorized,
                gin.H{"error": "Invalid webhook signature"})
            return
        }
 
        // Parse event
        var event services.StitchWebhookEvent
        if err := json.Unmarshal(rawBody, &event); err != nil {
            c.JSON(http.StatusBadRequest,
                gin.H{"error": "Invalid webhook payload"})
            return
        }
 
        // Handle event — identical to your current code
        if err := paymentService.HandleWebhook(&event); err != nil {
            log.Printf("stitch webhook: error handling event %q: %v",
                event.Type, err)
            c.JSON(http.StatusOK,
                gin.H{"received": true, "warning": err.Error()})
            return
        }
 
        c.JSON(http.StatusOK, gin.H{"received": true})
    }
}
 
// verifyStitchSignature() — deleted entirely.
// Svix SDK handles everything above.