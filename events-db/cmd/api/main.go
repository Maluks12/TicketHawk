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

	"events-db/internal/cache"
	"events-db/internal/config"
	"events-db/internal/controllers"
	"events-db/internal/database"
	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/ratelimit"
	"events-db/internal/repository"
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

	var rdb *redis.Client
	if rateLimitManager != nil {
		rdb = rateLimitManager.Client()
	}

	var eventCache *cache.EventCache
	if rdb != nil {
		eventCache = cache.NewEventCache(rdb)
		log.Info().Msg("event cache initialised")
	}

	// ==========================================
	// Workers
	// ==========================================
	workers.StartExpiryWorker(pool, eventCache, ctx, 15*time.Minute, 1*time.Minute)
	log.Info().Msg("booking expiry worker started")

	bookingJobs := make(chan string, 256)

	unsentIDs, err := repository.GetUnsentConfirmedBookingIDs(pool)
	if err != nil {
		log.Warn().Err(err).Msg("startup: failed to query unsent bookings — skipping re-enqueue")
	} else {
		for _, id := range unsentIDs {
			bookingJobs <- id
		}
		log.Info().Int("count", len(unsentIDs)).Msg("startup: re-enqueued unsent booking jobs")
	}
	workers.StartBookingWorker(pool, cfg, bookingJobs, ctx)

	// Ad nightly worker — resumes exhausted campaigns at midnight, ends expired ones
	workers.StartAdWorker(pool, ctx)
	log.Info().Msg("ad worker started")

	bookingRepo := repository.NewPgxBookingQuerier(pool)
	eventRepo := repository.NewPgxEventQuerier(pool)

	// ==========================================
	// Services
	// ==========================================
	emailService := services.NewEmailService(cfg)
	qrService := services.NewQRService(pool, cfg)
	paymentService := services.NewPaymentService(pool, cfg, bookingJobs)
	imgService := services.NewImageService(cfg)

	// ==========================================
	// Setup Router
	// ==========================================
	router := gin.New()
	router.SetTrustedProxies(nil)

	router.Use(middleware.RequestID())
	router.Use(middleware.Logger())
	router.Use(gin.Recovery())

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{cfg.FrontendURL},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Origin",
			"Authorization",
			"Content-Type",
			"X-Request-ID",
			"X-Session-ID",
		},
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
	// Home Feed
	// ==========================================
	router.GET("/home", middleware.OptionalAuth(cfg), handlers.HomeSectionsHandler(pool, eventCache))

	// ==========================================
	// Authentication Routes
	// ==========================================
	auth := router.Group("/auth")
	if rateLimitManager != nil {
		loginGroup := auth.Group("")
		loginGroup.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 5, RefillRate: 1.0 / 3600.0, TTL: 2 * time.Hour},
			"login"))
		loginGroup.POST("/login", handlers.LoginHandler(pool, cfg))

		registerGroup := auth.Group("")
		registerGroup.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 3600.0, TTL: 2 * time.Hour},
			"register"))
		registerGroup.POST("/register", handlers.CreateUserhandler(pool))

		passwordResetGroup := auth.Group("")
		passwordResetGroup.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 300.0, TTL: 1 * time.Hour},
			"password-reset"))
		passwordResetGroup.POST("/reset_password_request", handlers.RequestPasswordResetHandler(pool, emailService, cfg))
		passwordResetGroup.POST("/reset_password", handlers.ResetPasswordHandler(pool))
	} else {
		auth.POST("/login", handlers.LoginHandler(pool, cfg))
		auth.POST("/register", handlers.CreateUserhandler(pool))
		auth.POST("/reset_password_request", handlers.RequestPasswordResetHandler(pool, emailService, cfg))
		auth.POST("/reset_password", handlers.ResetPasswordHandler(pool))
	}

	if rateLimitManager != nil {
		refreshGroup := auth.Group("")
		refreshGroup.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 3600.0, TTL: 1 * time.Hour},
			"refresh"))
		refreshGroup.POST("/refresh", handlers.RefreshTokenHandler(pool, cfg))
	} else {
		auth.POST("/refresh", handlers.RefreshTokenHandler(pool, cfg))
	}
	auth.POST("/logout", middleware.AuthMiddleware(cfg), handlers.LogoutHandler(pool))

	// ==========================================
	// Public Event Routes
	// ==========================================
	public := router.Group("/events")
	public.Use(middleware.OptionalAuth(cfg))
	if rateLimitManager != nil {
		public.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 5.0, TTL: 15 * time.Minute},
			"events-public"))
	}
	{
		public.GET("/trending", handlers.TrendingEventHandlers(pool, eventCache))
		public.GET("", handlers.GetAllEventsHandlers(pool, eventCache))
		public.GET("/featured", handlers.FeaturedEventsHandlers(pool, eventCache))
		public.GET("/active", handlers.ActiveEventsHandlers(pool, eventCache))
		public.GET("/search", handlers.SearchEventsHandler(pool))
		public.GET("/category/:id", handlers.GetEventCategory(pool, eventCache))
		public.GET("/:slug", handlers.GetEventBySlugHandler(pool, eventCache))
	}

	// ==========================================
	// Public Merch Routes
	// ==========================================
	merch := router.Group("/events")
	merch.Use(middleware.OptionalAuth(cfg))
	if rateLimitManager != nil {
		merch.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 5.0, TTL: 15 * time.Minute},
			"merch-public"))
	}
	{
		merch.GET("/:id/merch", handlers.GetEventMerchHandler(pool, eventCache))
		merch.GET("/:id/merch/preview", handlers.GetEventMerchPreviewHandler(pool, eventCache))
	}

	// ==========================================
	// Single Merch Item Route
	// ==========================================
	merchItem := router.Group("/merch")
	merchItem.Use(middleware.OptionalAuth(cfg))
	if rateLimitManager != nil {
		merchItem.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 5.0, TTL: 15 * time.Minute},
			"merch-item"))
	}
	{
		merchItem.GET("/:id", handlers.GetMerchByIDHandler(pool, eventCache))
	}

	// ==========================================
	// Cart Routes
	// ==========================================
	cart := router.Group("/cart")
	if rateLimitManager != nil {
		cart.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"cart"))
	}
	{
		cart.GET("", middleware.OptionalAuth(cfg), handlers.GetCartHandler(pool))
		cart.POST("/items", middleware.OptionalAuth(cfg), handlers.AddCartItemHandler(pool))
		cart.PATCH("/items/:id", middleware.OptionalAuth(cfg), handlers.UpdateCartItemHandler(pool))
		cart.DELETE("/items/:id", middleware.OptionalAuth(cfg), handlers.DeleteCartItemHandler(pool))
		cart.DELETE("", middleware.OptionalAuth(cfg), handlers.ClearCartHandler(pool))
		cart.POST("/merge", middleware.AuthMiddleware(cfg), handlers.MergeCartHandler(pool))
	}

	// ==========================================
	// Checkout Routes
	// ==========================================
	checkout := router.Group("/checkout")
	if rateLimitManager != nil {
		checkout.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"checkout"))
	}
	{
		checkout.POST("/begin", middleware.OptionalAuth(cfg), handlers.CheckoutBeginHandler(pool))
		checkout.POST("/heartbeat", middleware.OptionalAuth(cfg), handlers.CheckoutHeartbeatHandler(pool))
		checkout.POST("/abandon", middleware.OptionalAuth(cfg), handlers.CheckoutAbandonHandler(pool))
		checkout.POST("/confirm", middleware.OptionalAuth(cfg), handlers.CheckoutConfirmHandler(pool, paymentService))
	}

	// ==========================================
	// Guest Booking Routes
	// ==========================================
	guest := router.Group("/guest")
	if rateLimitManager != nil {
		guest.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"guest-bookings"))
	}
	{
		guest.POST("/bookings", handlers.CreateGuestBookingHandler(pool, paymentService))
		guest.GET("/bookings/:bookingNumber", handlers.GetGuestBookingHandler(pool, rdb))
		guest.GET("/bookings/:bookingNumber/assignments", handlers.GetGuestTicketAssignmentsHandler(pool, rdb))
		guest.GET("/bookings/:bookingNumber/payment", handlers.GuestPaymentStatusHandler(pool, paymentService))
		guest.POST("/bookings/:bookingNumber/payment/retry", handlers.GuestRetryPaymentHandler(pool, paymentService))
	}

	// ==========================================
	// Notification Routes
	// ==========================================
	notifs := router.Group("/notifications")
	notifs.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		notifs.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"notifications"))
	}
	{
		notifs.GET("", handlers.GetNotificationsHandler(pool))
		notifs.GET("/unread", handlers.GetUnreadCountHandler(pool))
		notifs.PATCH("/:id/read", handlers.MarkReadHandler(pool))
		notifs.POST("/read-all", handlers.MarkAllReadHandler(pool))
	}

	// ==========================================
	// Admin Routes
	// ==========================================
	admin := router.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	if rateLimitManager != nil {
		admin.Use(middleware.CompositeRateLimit(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 5.0, TTL: 1 * time.Hour},
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 10.0, TTL: 30 * time.Minute},
			"admin"))
	}
	{
		admin.POST("/events", handlers.CreateEventHandlers(pool, eventCache))
		admin.PUT("/events/:id", handlers.UpdateEventHandlers(pool, eventCache))
		admin.GET("/events/:id", handlers.GetEventByIDHandlers(pool, eventCache))
		admin.DELETE("/events/:id", handlers.DeleteEventHandlers(pool))
		admin.GET("/events", handlers.GetAdminEventsHandler(pool))
		admin.GET("/stats", handlers.GetAdminStatsHandler(pool))
		admin.GET("/merch/oversold", handlers.GetOversoldMerchOrdersHandler(pool))
		admin.GET("/users", controllers.GetAllUsersHandler(pool))
		admin.PUT("/users/promote/:id", controllers.PromoteUserHandler(pool))
		admin.PUT("/users/demote/:id", controllers.DemoteUserHandler(pool))
		admin.DELETE("/users/delete/:id", handlers.DeleteUserHandler(pool))

		// Ad campaign oversight
		admin.GET("/campaigns", handlers.AdminGetCampaignsHandler(pool))
	}

	// ==========================================
	// Organiser Routes
	// ==========================================
	orgMerch := router.Group("/organiser")
	orgMerch.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	if rateLimitManager != nil {
		orgMerch.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 50, RefillRate: 2.0, TTL: 15 * time.Minute},
			"organiser-merch"))
	}
	{
		// Event management
		orgMerch.POST("/events/:id/publish", handlers.PublishEventHandler(pool, eventCache))
		orgMerch.POST("/events/:id/cancel", handlers.CancelEventHandler(pool, eventCache))
		orgMerch.GET("/events", handlers.GetOrganizerEventsHandler(pool))

		// Merch
		orgMerch.POST("/events/:id/merch", handlers.OrgCreateMerchHandler(pool, eventCache))
		orgMerch.PUT("/merch/:id", handlers.OrgUpdateMerchHandler(pool, eventCache))
		orgMerch.DELETE("/merch/:id", handlers.OrgDeactivateMerchHandler(pool, eventCache))
		orgMerch.POST("/merch/:id/variants", handlers.OrgCreateVariantHandler(pool, eventCache))
		orgMerch.PUT("/merch/variants/:id", handlers.OrgUpdateVariantHandler(pool, eventCache))

		// ── Ad wallet & campaigns ──────────────────────────────────────────
		orgMerch.GET("/ad-wallet", handlers.GetWalletHandler(pool))
		orgMerch.POST("/ad-wallet/topup", handlers.TopupWalletHandler(pool, paymentService))
		orgMerch.POST("/events/:id/campaigns", handlers.CreateCampaignHandler(pool))
		orgMerch.GET("/campaigns", handlers.GetMyCampaignsHandler(pool))
		orgMerch.GET("/campaigns/:id", handlers.GetCampaignHandler(pool))
		orgMerch.PATCH("/campaigns/:id", handlers.UpdateCampaignHandler(pool))
		orgMerch.DELETE("/campaigns/:id", handlers.EndCampaignHandler(pool))
	}

	// ==========================================
	// Rate Limit Management (Admin)
	// ==========================================
	if rateLimitManager != nil {
		adminAPI := router.Group("/admin/ratelimit")
		adminAPI.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
		{
			adminAPI.GET("/ip/:ip", handlers.RateLimitIPStatusHandler(rateLimitManager))
			adminAPI.POST("/ip/:ip/reset", handlers.RateLimitIPResetHandler(rateLimitManager))
			adminAPI.GET("/user/:userId", handlers.RateLimitUserStatusHandler(rateLimitManager))
			adminAPI.POST("/user/:userId/reset", handlers.RateLimitUserResetHandler(rateLimitManager))
			adminAPI.GET("/metrics", handlers.RateLimitMetricsHandler())
		}
	}

	// ==========================================
	// Media Upload Routes
	// ==========================================
	media := router.Group("/organiser/events/:id/media")
	media.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	if rateLimitManager != nil {
		media.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"media-upload"))
	}
	{
		media.POST("/cover", handlers.UploadCoverImageHandler(pool, imgService))
		media.POST("/banner", handlers.UploadBannerImageHandler(pool, imgService))
		media.POST("/banner/video", handlers.UploadBannerVideoHandler(pool, imgService))
		media.DELETE("/banner/video", handlers.DeleteBannerVideoHandler(pool, imgService))
		media.POST("/gallery", handlers.UploadGalleryImageHandler(pool, imgService))
	}

	// ==========================================
	// Booking Routes
	// ==========================================
	bookings := router.Group("/bookings")
	bookings.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		bookings.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"bookings"))
	}
	{
		bookings.POST("", handlers.CreateBookingHandler(bookingRepo, eventRepo, paymentService))
		bookings.GET("", handlers.GetUserBookingsHandler(bookingRepo))
		bookings.GET("/:id", handlers.GetBookingDetailsHandler(bookingRepo))
		bookings.POST("/:id/refund", handlers.RequestRefundHandler(bookingRepo, eventRepo, paymentService, emailService))
		bookings.GET("/:id/assignments", handlers.GetTicketAssignmentsHandler(pool))
		bookings.POST("/:id/verify", handlers.VerifyQRCodeHandler(bookingRepo, eventRepo, qrService))
	}

	// ==========================================
	// User Profile Routes
	// ==========================================
	user := router.Group("/user")
	user.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		user.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"user-profile"))
	}
	{
		user.GET("/me", handlers.GetMyProfileHandler(pool))
		user.GET("/merch-orders", handlers.GetMyMerchOrdersHandler(pool))
	}

	// ==========================================
	// Payment Routes
	// ==========================================
	payments := router.Group("/bookings")
	payments.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		payments.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"payments"))
	}
	{
		payments.POST("/:id/payment", handlers.CreateStitchPayment(pool, paymentService))
		payments.GET("/:id/payment", handlers.GetPaymentStatusHandler(pool, paymentService))
		payments.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentService))
	}

	// ==========================================
	// Webhook Routes
	// ==========================================
	webhooks := router.Group("/webhooks")
	if rateLimitManager != nil {
		webhooks.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 10.0, TTL: 1 * time.Minute},
			"webhooks"))
	}
	{
		// Booking payments
		webhooks.POST("/stitch", handlers.StitchWebhookHandler(paymentService, cfg.StitchWebhookSecret))
		// Ad wallet top-ups — separate handler, separate endpoint
		webhooks.POST("/stitch-ads", handlers.AdTopupWebhookHandler(pool, cfg.StitchWebhookSecret))
	}

	// ==========================================
	// Protected Test Route
	// ==========================================
	router.GET("/protected-test", middleware.AuthMiddleware(cfg), handlers.TestProtectedHandler())

	// ==========================================
	// Start Server — Graceful shutdown
	// ==========================================
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	log.Info().
		Str("port", cfg.Port).
		Bool("rate_limiting", rateLimitManager != nil).
		Str("email_from", cfg.EmailFromName+" <"+cfg.EmailFromAddress+">").
		Str("frontend_url", cfg.FrontendURL).
		Msg("server starting")

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()
	<-ctx.Done()
	stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	} else {
		log.Info().Msg("server exited cleanly")
	}
}