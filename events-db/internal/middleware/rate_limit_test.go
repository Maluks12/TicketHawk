package middleware

// import (
// 	//"fmt"
// 	"context"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	"time"

// 	"events-db/internal/ratelimit"

// 	"github.com/gin-gonic/gin"
// 	"github.com/redis/go-redis/v9"
// 	"github.com/stretchr/testify/assert"
// )

// func setupTestManager(t *testing.T) *ratelimit.Manager {
// 	client := redis.NewClient(&redis.Options{
// 		Addr: "localhost:6379",
// 	})

// 	ctx := context.Background()
// 	if err := client.Ping(ctx).Err(); err != nil {
// 		t.Skip("Redis not available")
// 	}

// 	client.FlushDB(ctx)

// 	manager, _ := ratelimit.NewManager("localhost:6379")
// 	return manager
// }

// func TestRateLimitByIP_Middleware(t *testing.T) {
// 	manager := setupTestManager(t)
// 	defer manager.Close()

// 	config := ratelimit.BucketConfig{
// 		Capacity:   3,
// 		RefillRate: 1.0,
// 		TTL:        1 * time.Minute,
// 	}

// 	gin.SetMode(gin.TestMode)
// 	r := gin.New()
// 	r.Use(RateLimitByIP(manager, config, "test"))
// 	r.GET("/test", func(c *gin.Context) {
// 		c.JSON(200, gin.H{"ok": true})
// 	})

// 	// First 3 requests should succeed
// 	for i := 0; i < 3; i++ {
// 		w := httptest.NewRecorder()
// 		req := httptest.NewRequest("GET", "/test", nil)
// 		req.RemoteAddr = "192.168.1.1:1234"
		
// 		r.ServeHTTP(w, req)
		
// 		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i+1)
// 		assert.Contains(t, w.Header().Get("X-RateLimit-Limit"), "3")
// 	}

// 	// 4th request should be rate limited
// 	w := httptest.NewRecorder()
// 	req := httptest.NewRequest("GET", "/test", nil)
// 	req.RemoteAddr = "192.168.1.1:1234"
	
// 	r.ServeHTTP(w, req)
	
// 	assert.Equal(t, http.StatusTooManyRequests, w.Code)
// 	assert.NotEmpty(t, w.Header().Get("Retry-After"))
// }

// func TestRateLimitByUser_Middleware(t *testing.T) {
// 	manager := setupTestManager(t)
// 	defer manager.Close()

// 	config := ratelimit.BucketConfig{
// 		Capacity:   5,
// 		RefillRate: 1.0,
// 		TTL:        1 * time.Minute,
// 	}

// 	gin.SetMode(gin.TestMode)
// 	r := gin.New()
	
// 	// Middleware to set user_id
// 	r.Use(func(c *gin.Context) {
// 		c.Set("user_id", "user123")
// 		c.Next()
// 	})
	
// 	r.Use(RateLimitByUser(manager, config, "test"))
// 	r.GET("/test", func(c *gin.Context) {
// 		c.JSON(200, gin.H{"ok": true})
// 	})

// 	// First 5 requests should succeed
// 	for i := 0; i < 5; i++ {
// 		w := httptest.NewRecorder()
// 		req := httptest.NewRequest("GET", "/test", nil)
		
// 		r.ServeHTTP(w, req)
		
// 		assert.Equal(t, http.StatusOK, w.Code)
// 	}

// 	// 6th request should be rate limited
// 	w := httptest.NewRecorder()
// 	req := httptest.NewRequest("GET", "/test", nil)
	
// 	r.ServeHTTP(w, req)
	
// 	assert.Equal(t, http.StatusTooManyRequests, w.Code)
// }

// func TestDifferentIPsIndependent(t *testing.T) {
// 	manager := setupTestManager(t)
// 	defer manager.Close()

// 	config := ratelimit.BucketConfig{
// 		Capacity:   2,
// 		RefillRate: 1.0,
// 		TTL:        1 * time.Minute,
// 	}

// 	gin.SetMode(gin.TestMode)
// 	r := gin.New()
// 	r.Use(RateLimitByIP(manager, config, "test"))
// 	r.GET("/test", func(c *gin.Context) {
// 		c.JSON(200, gin.H{"ok": true})
// 	})

// 	// Exhaust IP1
// 	for i := 0; i < 2; i++ {
// 		w := httptest.NewRecorder()
// 		req := httptest.NewRequest("GET", "/test", nil)
// 		req.RemoteAddr = "192.168.1.1:1234"
// 		r.ServeHTTP(w, req)
// 	}

// 	// IP1 should be rate limited
// 	w1 := httptest.NewRecorder()
// 	req1 := httptest.NewRequest("GET", "/test", nil)
// 	req1.RemoteAddr = "192.168.1.1:1234"
// 	r.ServeHTTP(w1, req1)
// 	assert.Equal(t, http.StatusTooManyRequests, w1.Code)

// 	// IP2 should still work
// 	w2 := httptest.NewRecorder()
// 	req2 := httptest.NewRequest("GET", "/test", nil)
// 	req2.RemoteAddr = "192.168.1.2:1234"
// 	r.ServeHTTP(w2, req2)
// 	assert.Equal(t, http.StatusOK, w2.Code)
// }