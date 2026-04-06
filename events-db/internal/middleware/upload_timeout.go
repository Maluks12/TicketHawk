package middleware

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

// ExtendWriteTimeout overrides the connection's write deadline for routes that
// need more time than the server-level WriteTimeout (e.g. file uploads).
// Pass the desired duration, e.g. 5 * time.Minute.
//
// Usage in main.go:
//
//	media.Use(middleware.ExtendWriteTimeout(5 * time.Minute))
func ExtendWriteTimeout(d time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        rc := http.NewResponseController(c.Writer)
        _ = rc.SetWriteDeadline(time.Now().Add(d))
        c.Next()
    }
}