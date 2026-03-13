package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"events-db/internal/config"
	"events-db/internal/controllers"
	"events-db/internal/database"
	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/ratelimit"
	"events-db/internal/services"
	"events-db/internal/workers"
	"github.com/gin-contrib/cors"
)

func main() {

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if os.Getenv("ENV") != "production" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// ==========================================
	// Load Configuration
	// ==========================================
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	// ==========================================
	// Connect to Database
	// ==========================================
	var pool *pgxpool.Pool
	pool, err = database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	log.Info().Msg("database connected successfully")

	// ==========================================
	// Signal Context (shared by server + workers)
	// ==========================================
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ==========================================
	// Booking Expiry Worker
	// ==========================================
	workers.StartExpiryWorker(pool, ctx, 15*time.Minute, 1*time.Minute)
	log.Info().Msg("booking expiry worker started")

	bookingJobs := make(chan string, 256)
	workers.StartBookingWorker(pool, cfg, bookingJobs, ctx)
	// ==========================================
	// Services
	// ==========================================
	emailService := services.NewEmailService(cfg)
	qrService := services.NewQRService(pool, cfg)
	paymentService := services.NewPaymentService(pool, cfg, bookingJobs)

	// ==========================================
	// Initialize Rate Limit Manager
	// ==========================================
	rateLimitManager, err := ratelimit.NewManager(cfg.RedisAddr)
	if err != nil {
		log.Warn().Err(err).Msg("rate limiter unavailable — rate limiting DISABLED")
		rateLimitManager = nil
	} else {
		log.Info().Msg("rate limiter initialized with Redis")
		defer rateLimitManager.Close()
	}

	// FIX 2 — nil-guard the Redis client before passing it into the guest
	// handler. If Redis is down, rateLimitManager is nil and calling
	// .Client() on nil would panic at startup. The handler accepts a nil
	// *redis.Client and degrades gracefully (lockout counter just disabled).
	var rdb *redis.Client
	if rateLimitManager != nil {
		rdb = rateLimitManager.Client()
	}

	// ==========================================
	// Setup Router
	// ==========================================
	router := gin.New()
	router.SetTrustedProxies(nil)

	router.Use(middleware.RequestID())
	router.Use(middleware.Logger())
	router.Use(gin.Recovery())

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.FrontendURL},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// ==========================================
	// Global IP Rate Limiting
	// ==========================================
	if rateLimitManager != nil {
		router.Use(middleware.GlobalIPRateLimit(rateLimitManager))
		log.Info().Msg("global IP rate limiting enabled")
	}

	// ==========================================
	// Health Check
	// ==========================================
	router.GET("/health", func(c *gin.Context) {
		status := gin.H{"status": "ok"}
		code := http.StatusOK

		if err := pool.Ping(c.Request.Context()); err != nil {
			status["postgres"] = "unhealthy: " + err.Error()
			code = http.StatusServiceUnavailable
		} else {
			status["postgres"] = "ok"
		}

		if rateLimitManager != nil {
			if err := rateLimitManager.Ping(c.Request.Context()); err != nil {
				status["redis"] = "unhealthy"
			} else {
				status["redis"] = "ok"
			}
		}

		c.JSON(code, status)
	})

	// ==========================================
	// Authentication Routes (Strict rate limiting by IP)
	// ==========================================
	auth := router.Group("/auth")
	if rateLimitManager != nil {
		// Login — 5 attempts per hour per IP
		loginGroup := auth.Group("")
		loginGroup.Use(middleware.RateLimitByIP(
			rateLimitManager,
			ratelimit.BucketConfig{
				Capacity:   5,
				RefillRate: 1.0 / 3600.0,
				TTL:        2 * time.Hour,
			},
			"login",
		))
		loginGroup.POST("/login", handlers.LoginHandler(pool, cfg))

		// Register — 3 registrations per hour per IP
		registerGroup := auth.Group("")
		registerGroup.Use(middleware.RateLimitByIP(
			rateLimitManager,
			ratelimit.BucketConfig{
				Capacity:   3,
				RefillRate: 1.0 / 3600.0,
				TTL:        2 * time.Hour,
			},
			"register",
		))
		registerGroup.POST("/register", handlers.CreateUserhandler(pool))

		// Password reset — 3 attempts per 5 minutes per IP
		passwordResetGroup := auth.Group("")
		passwordResetGroup.Use(middleware.RateLimitByIP(
			rateLimitManager,
			ratelimit.BucketConfig{
				Capacity:   3,
				RefillRate: 1.0 / 300.0,
				TTL:        1 * time.Hour,
			},
			"password-reset",
		))
		passwordResetGroup.POST("/reset_password_request", handlers.RequestPasswordResetHandler(pool, emailService, cfg))
		passwordResetGroup.POST("/reset_password", handlers.ResetPasswordHandler(pool))
	} else {
		// Fallback — no rate limiting
		auth.POST("/login", handlers.LoginHandler(pool, cfg))
		auth.POST("/register", handlers.CreateUserhandler(pool))
		auth.POST("/reset_password_request", handlers.RequestPasswordResetHandler(pool, emailService, cfg))
		auth.POST("/reset_password", handlers.ResetPasswordHandler(pool))
	}

	// FIX 3 — refresh and logout moved outside the if/else so they are
	// always registered regardless of whether rate limiting is enabled.
	auth.POST("/refresh", handlers.RefreshTokenHandler(pool, cfg))
	auth.POST("/logout", middleware.AuthMiddleware(cfg), handlers.LogoutHandler(pool))

	// ==========================================
	// Public Event Routes
	// ==========================================
	public := router.Group("/events")
	if rateLimitManager != nil {
		public.Use(middleware.RateLimitByUser(
			rateLimitManager,
			ratelimit.BucketConfig{
				Capacity:   200,
				RefillRate: 5.0,
				TTL:        15 * time.Minute,
			},
			"events-public",
		))
	}
	{
		public.GET("/trending", handlers.TrendingEventHandlers(pool))
		public.GET("", handlers.GetAllEventsHandlers(pool))
		public.GET("/featured", handlers.FeaturedEventsHandlers(pool))
		public.GET("/active", handlers.ActiveEventsHandlers(pool))
		public.GET("/search", handlers.SearchEventsHandler(pool))
	}

	// ==========================================
	// Guest Booking Routes (no auth required)
	// ==========================================
	guest := router.Group("/guest")
	if rateLimitManager != nil {
		guest.Use(middleware.RateLimitByIP(
			rateLimitManager,
			ratelimit.BucketConfig{
				Capacity:   10,
				RefillRate: 1.0 / 60.0,
				TTL:        15 * time.Minute,
			},
			"guest-bookings",
		))
	}
	{
		guest.POST("/bookings", handlers.CreateGuestBookingHandler(pool, paymentService))
		guest.GET("/bookings/:bookingNumber", handlers.GetGuestBookingHandler(pool, rdb)) // uses nil-safe rdb
		guest.GET("/bookings/:bookingNumber/payment", handlers.GuestPaymentStatusHandler(pool, paymentService))
		guest.POST("/bookings/:bookingNumber/payment/retry", handlers.GuestRetryPaymentHandler(pool, paymentService))
	}

	// ==========================================
	// Admin Routes (Composite rate limiting: IP + User)
	// ==========================================
	admin := router.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	if rateLimitManager != nil {
		admin.Use(middleware.CompositeRateLimit(
			rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 5.0, TTL: 1 * time.Hour},
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 10.0, TTL: 30 * time.Minute},
			"admin",
		))
	}
	{
		admin.POST("/events", handlers.CreateEventHandlers(pool))
		admin.PUT("/events/:id", handlers.UpdateEventHandlers(pool))
		admin.GET("/events/:id", handlers.GetEventByIDHandlers(pool))
		admin.GET("/events/category/:id", handlers.GetEventCategory(pool))
		admin.DELETE("/events/:id", handlers.DeleteEventHandlers(pool))

		admin.GET("/users", controllers.GetAllUsersHandler(pool))
		admin.PUT("/users/promote/:id", controllers.PromoteUserHandler(pool))
		admin.PUT("/users/demote/:id", controllers.DemoteUserHandler(pool))
		admin.DELETE("/users/delete/:id", handlers.DeleteUserHandler(pool))
	}

	// ==========================================
	// Rate Limit Management Endpoints (Admin only)
	// ==========================================
	if rateLimitManager != nil {
		adminAPI := router.Group("/admin/ratelimit")
		adminAPI.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
		{
			adminAPI.GET("/ip/:ip", func(c *gin.Context) {
				ip := c.Param("ip")
				category := c.DefaultQuery("category", "global")
				key := ratelimit.IPKey(category, ip)

				var bcfg ratelimit.BucketConfig
				switch category {
				case "login":
					bcfg = ratelimit.BucketConfig{Capacity: 5, RefillRate: 1.0 / 3600.0, TTL: 2 * time.Hour}
				case "register":
					bcfg = ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 3600.0, TTL: 2 * time.Hour}
				case "password-reset":
					bcfg = ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 300.0, TTL: 1 * time.Hour}
				default:
					bcfg = ratelimit.GlobalIPBucket
				}

				limiter := rateLimitManager.GetLimiter(category, bcfg)
				tokens, err := limiter.Peek(c.Request.Context(), key)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check rate limit"})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"ip": ip, "category": category,
					"tokens": tokens, "capacity": bcfg.Capacity, "status": tokens > 0,
				})
			})

			adminAPI.POST("/ip/:ip/reset", func(c *gin.Context) {
				ip := c.Param("ip")
				category := c.DefaultQuery("category", "global")
				key := ratelimit.IPKey(category, ip)

				var bcfg ratelimit.BucketConfig
				switch category {
				case "login":
					bcfg = ratelimit.BucketConfig{Capacity: 5, RefillRate: 1.0 / 3600.0, TTL: 2 * time.Hour}
				case "register":
					bcfg = ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 3600.0, TTL: 2 * time.Hour}
				default:
					bcfg = ratelimit.GlobalIPBucket
				}

				limiter := rateLimitManager.GetLimiter(category, bcfg)
				if err := limiter.Reset(c.Request.Context(), key); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset rate limit"})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"message": "Rate limit reset successfully", "ip": ip, "category": category,
				})
			})

			adminAPI.GET("/user/:userId", func(c *gin.Context) {
				userID := c.Param("userId")
				category := c.DefaultQuery("category", "events-read")
				key := ratelimit.UserKey(category, userID)

				var bcfg ratelimit.BucketConfig
				switch category {
				case "events-read":
					bcfg = ratelimit.BucketConfig{Capacity: 100, RefillRate: 2.0, TTL: 15 * time.Minute}
				case "admin":
					bcfg = ratelimit.BucketConfig{Capacity: 200, RefillRate: 10.0, TTL: 30 * time.Minute}
				default:
					bcfg = ratelimit.APIReadBucket
				}

				limiter := rateLimitManager.GetLimiter(category, bcfg)
				tokens, err := limiter.Peek(c.Request.Context(), key)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check rate limit"})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"user_id": userID, "category": category,
					"tokens": tokens, "capacity": bcfg.Capacity, "status": tokens > 0,
				})
			})

			adminAPI.POST("/user/:userId/reset", func(c *gin.Context) {
				userID := c.Param("userId")
				category := c.DefaultQuery("category", "events-read")
				key := ratelimit.UserKey(category, userID)

				bcfg := ratelimit.BucketConfig{Capacity: 100, RefillRate: 2.0, TTL: 15 * time.Minute}
				limiter := rateLimitManager.GetLimiter(category, bcfg)
				if err := limiter.Reset(c.Request.Context(), key); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset rate limit"})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"message": "Rate limit reset successfully", "user_id": userID, "category": category,
				})
			})

			adminAPI.GET("/metrics", func(c *gin.Context) {
				metrics := ratelimit.GetMetrics()
				c.JSON(http.StatusOK, gin.H{
					"total_requests":   metrics.TotalRequests,
					"allowed_requests": metrics.AllowedRequests,
					"denied_requests":  metrics.DeniedRequests,
					"errors":           metrics.Errors,
					"allow_rate":       metrics.AllowRate(),
					"categories":       metrics.CategoryMetrics,
				})
			})
		}
	}

	// ==========================================
	// Booking Routes (Auth required)
	// ==========================================
	bookings := router.Group("/bookings")
	bookings.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		bookings.Use(middleware.RateLimitByUser(
			rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"bookings",
		))
	}
	{
		bookings.POST("", handlers.CreateBookingHandler(pool, paymentService))
		bookings.GET("", handlers.GetUserBookingsHandler(pool))
		bookings.GET("/:id", handlers.GetBookingDetailsHandler(pool))
		bookings.POST("/:id/refund", handlers.RequestRefundHandler(pool, paymentService))
		bookings.POST("/:id/verify", handlers.VerifyQRCodeHandler(pool, qrService))
	}

	// ==========================================
	// Payment Routes (Auth required, stricter limits)
	// ==========================================
	payments := router.Group("/bookings")
	payments.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		payments.Use(middleware.RateLimitByUser(
			rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"payments",
		))
	}
	{
		payments.POST("/:id/payment", handlers.CreateStitchPayment(pool, paymentService))
		payments.GET("/:id/payment", handlers.GetPaymentStatusHandler(pool, paymentService))
		payments.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentService))
	}

	// ==========================================
	// Webhook Routes (No auth — called by Stitch)
	// ==========================================
	webhooks := router.Group("/webhooks")
	if rateLimitManager != nil {
		webhooks.Use(middleware.RateLimitByIP(
			rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 10.0, TTL: 1 * time.Minute},
			"webhooks",
		))
	}
	{
		webhooks.POST("/stitch", handlers.StitchWebhookHandler(paymentService, cfg.StitchWebhookSecret))
	}

	// ==========================================
	// Protected Test Route
	// ==========================================
	router.GET("/protected-test", middleware.AuthMiddleware(cfg), handlers.TestProtectedHandler())
	// ==========================================
	// Start Server — Step 8: Graceful shutdown
	// ==========================================
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Info().
		Str("port", cfg.Port).
		Bool("rate_limiting", rateLimitManager != nil).
		Str("email_from", cfg.EmailFromName+" <"+cfg.EmailFromAddress+">").
		Str("frontend_url", cfg.FrontendURL).
		Msg("server starting")

	// Start listening in the background
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()
	<-ctx.Done()
	stop() // release the signal notify context

	// Give in-flight requests 30 seconds to finish
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	} else {
		log.Info().Msg("server exited cleanly")
	}
}