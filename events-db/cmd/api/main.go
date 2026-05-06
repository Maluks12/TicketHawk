package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

	workers.StartAdWorker(pool, ctx)
	log.Info().Msg("ad worker started")

	// FIX 3: instantiate all repository queriers up front so both the booking
	// handlers and the checkout handlers receive interfaces, not a raw pool.
	bookingRepo    := repository.NewPgxBookingQuerier(pool)
	eventRepo      := repository.NewPgxEventQuerier(pool)
	cartQuerier    := repository.NewPgxCartQuerier(pool)
	reserveQuerier := repository.NewPgxReservationQuerier(pool)

	// ==========================================
	// Services
	// ==========================================
	emailService   := services.NewEmailService(cfg)
	qrService      := services.NewQRService(pool, cfg)
	paymentService := services.NewPaymentService(pool, cfg, bookingJobs)
	imgService     := services.NewImageService(cfg)

	// ==========================================
	// Setup Router
	// ==========================================
	router := gin.New()
	router.SetTrustedProxies(nil)

	router.Use(middleware.RequestID())
	router.Use(middleware.Logger())
	router.Use(gin.Recovery())

	router.Use(cors.New(cors.Config{
		AllowOrigins: cfg.FrontendURLs,
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
	// Health Check (intentionally not versioned — infra tools call this)
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
	// Webhook Routes (intentionally not versioned — Stitch calls these)
	// ==========================================
	webhooks := router.Group("/webhooks")
	if rateLimitManager != nil {
		webhooks.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 10.0, TTL: 1 * time.Minute},
			"webhooks"))
	}
	{
		webhooks.POST("/stitch",     handlers.StitchWebhookHandler(paymentService, cfg.StitchWebhookSecret))
		webhooks.POST("/stitch-ads", handlers.AdTopupWebhookHandler(pool, cfg.StitchWebhookSecret))
	}

	// ==========================================
	// API v1
	// ==========================================
	v1 := router.Group("/v1")

	// ==========================================
	// Authentication Routes
	// ==========================================
	auth := v1.Group("/auth")
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
		passwordResetGroup.POST("/reset_password",         handlers.ResetPasswordHandler(pool))
	} else {
		auth.POST("/login",                    handlers.LoginHandler(pool, cfg))
		auth.POST("/register",                 handlers.CreateUserhandler(pool))
		auth.POST("/reset_password_request",   handlers.RequestPasswordResetHandler(pool, emailService, cfg))
		auth.POST("/reset_password",           handlers.ResetPasswordHandler(pool))
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
	public := v1.Group("/events")
	public.Use(middleware.OptionalAuth(cfg))
	if rateLimitManager != nil {
		public.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 5.0, TTL: 15 * time.Minute},
			"events-public"))
	}
	{
		public.GET("/trending",          handlers.TrendingEventHandler(pool, eventCache))
		public.GET("",                   handlers.GetAllEventsHandler(pool, eventCache))
		public.GET("/featured",          handlers.FeaturedEventsHandler(pool, eventCache))
		public.GET("/active",            handlers.ActiveEventsHandler(pool, eventCache))
		public.GET("/search",            handlers.SearchEventsHandler(pool))
		public.GET("/category/:id",      handlers.GetEventCategoryHandler(pool, eventCache))
		public.GET("/filter",            handlers.FilterEventsHandler(pool))
		public.GET("/:id/merch",         handlers.GetEventMerchHandler(pool, eventCache))
		public.GET("/:id/merch/preview", handlers.GetEventMerchPreviewHandler(pool, eventCache))
		// Ticket types — visible tiers for the event detail page
		public.GET("/:id/ticket-types",  handlers.GetTicketTypesHandler(pool))
		// Must be last — catches both UUIDs and slugs
		public.GET("/:id",               handlers.GetEventBySlugHandler(pool, eventCache))
	}

	// ==========================================
	// City Lookup
	// ==========================================
	cities := v1.Group("/cities")
	if rateLimitManager != nil {
		cities.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"cities"))
	}
	{
		cities.GET("", handlers.CitiesHandler(pool))
	}

	// ==========================================
	// Single Merch Item Route
	// ==========================================
	merchItem := v1.Group("/merch")
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
	cart := v1.Group("/cart")
	if rateLimitManager != nil {
		cart.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"cart"))
	}
	{
		cart.GET("",              middleware.OptionalAuth(cfg), handlers.GetCartHandler(pool))
		cart.POST("/items",       middleware.OptionalAuth(cfg), handlers.AddCartItemHandler(pool))
		cart.PATCH("/items/:id",  middleware.OptionalAuth(cfg), handlers.UpdateCartItemHandler(pool))
		cart.DELETE("/items/:id", middleware.OptionalAuth(cfg), handlers.DeleteCartItemHandler(pool))
		cart.DELETE("",           middleware.OptionalAuth(cfg), handlers.ClearCartHandler(pool))
		cart.POST("/merge",       middleware.AuthMiddleware(cfg), handlers.MergeCartHandler(pool))
	}

	// ==========================================
	// Checkout Routes
	// ==========================================
	checkout := v1.Group("/checkout")
	if rateLimitManager != nil {
		checkout.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"checkout"))
	}
	{
		checkout.POST("/begin",     middleware.OptionalAuth(cfg), handlers.CheckoutBeginHandler(cartQuerier, reserveQuerier, eventRepo))
		checkout.POST("/heartbeat", middleware.OptionalAuth(cfg), handlers.CheckoutHeartbeatHandler(cartQuerier, reserveQuerier))
		checkout.POST("/abandon",   middleware.OptionalAuth(cfg), handlers.CheckoutAbandonHandler(cartQuerier, reserveQuerier))
		checkout.POST("/confirm",   middleware.OptionalAuth(cfg), handlers.CheckoutConfirmHandler(cartQuerier, reserveQuerier, bookingRepo, paymentService))
	}

	// ==========================================
	// Guest Booking Routes
	// ==========================================
	guest := v1.Group("/guest")
	if rateLimitManager != nil {
		guest.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"guest-bookings"))
	}
	{
		guest.POST("/bookings",                                  handlers.CreateGuestBookingHandler(pool, paymentService))
		guest.GET("/bookings/:bookingNumber",                    handlers.GetGuestBookingHandler(pool, rdb))
		guest.GET("/bookings/:bookingNumber/assignments",        handlers.GetGuestTicketAssignmentsHandler(pool, rdb))
		guest.GET("/bookings/:bookingNumber/payment",            handlers.GuestPaymentStatusHandler(pool, paymentService))
		guest.POST("/bookings/:bookingNumber/payment/retry",     handlers.GuestRetryPaymentHandler(pool, paymentService))
	}

	// ==========================================
	// Notification Routes
	// ==========================================
	notifs := v1.Group("/notifications")
	notifs.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		notifs.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"notifications"))
	}
	{
		notifs.GET("",              handlers.GetNotificationsHandler(pool))
		notifs.GET("/unread",       handlers.GetUnreadCountHandler(pool))
		notifs.PATCH("/:id/read",   handlers.MarkReadHandler(pool))
		notifs.POST("/read-all",    handlers.MarkAllReadHandler(pool))
	}

	// ==========================================
	// Bookings Routes
	// ==========================================
	bookings := v1.Group("/bookings")
	bookings.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		bookings.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"bookings"))
	}
	{
		bookings.POST("",      handlers.CreateBookingHandler(bookingRepo, eventRepo, paymentService, cfg))
		bookings.GET("",       handlers.GetUserBookingsHandler(bookingRepo))
		bookings.GET("/:id",   handlers.GetBookingDetailsHandler(bookingRepo))
		bookings.POST("/:id/refund",                        handlers.RequestRefundHandler(bookingRepo, eventRepo, paymentService, emailService))
		bookings.GET("/:id/assignments",                    handlers.GetTicketAssignmentsHandler(pool))
		bookings.POST("/:id/assignments/:assignmentId",     handlers.AssignTicketHandler(pool, emailService, cfg.FrontendURLs[0]))
		bookings.DELETE("/:id/assignments/:assignmentId",   handlers.RevokeAssignmentHandler(pool))
		bookings.POST("/:id/verify",                        handlers.VerifyQRCodeHandler(bookingRepo, eventRepo, qrService))

		if rateLimitManager != nil {
			paymentRoutes := bookings.Group("")
			paymentRoutes.Use(middleware.RateLimitByUser(rateLimitManager,
				ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
				"payments"))
			paymentRoutes.POST("/:id/payment",       handlers.CreateStitchPayment(pool, paymentService))
			paymentRoutes.GET("/:id/payment",        handlers.GetPaymentStatusHandler(pool, paymentService))
			paymentRoutes.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentService))
			bookings.GET("/:id/payment/page", handlers.PaymentPageHandler())
		} else {
			bookings.POST("/:id/payment",       handlers.CreateStitchPayment(pool, paymentService))
			bookings.GET("/:id/payment",        handlers.GetPaymentStatusHandler(pool, paymentService))
			bookings.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentService))
			bookings.GET("/:id/payment/page", handlers.PaymentPageHandler())
		}
	}

	// ==========================================
	// Ticket Claim Route (public — no auth)
	// ==========================================
	tickets := v1.Group("/tickets")
	if rateLimitManager != nil {
		tickets.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"ticket-claim"))
	}
	{
		tickets.POST("/claim", handlers.ClaimTicketHandler(pool, emailService))
	}

	// ==========================================
	// User Profile Routes
	// ==========================================
	user := v1.Group("/user")
	user.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		user.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"user-profile"))
	}
	{
		user.GET("/me",           handlers.GetMyProfileHandler(pool))
		user.GET("/merch-orders", handlers.GetMyMerchOrdersHandler(pool))
	}

	// ==========================================
	// Home Feed
	// ==========================================
	v1.GET("/home", middleware.OptionalAuth(cfg), handlers.HomeSectionsHandler(pool, eventCache))

	// ==========================================
	// Drops Feed
	// ==========================================
	v1.GET("/drops", handlers.GetDropsFeedHandler(pool))

	// ==========================================
	// Admin Routes
	// ==========================================
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	if rateLimitManager != nil {
		admin.Use(middleware.CompositeRateLimit(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 5.0, TTL: 1 * time.Hour},
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 10.0, TTL: 30 * time.Minute},
			"admin"))
	}
	{
		admin.POST("/events",        handlers.CreateEventHandler(pool, eventCache))
		admin.PUT("/events/:id",     handlers.UpdateEventHandler(pool, eventCache))
		admin.GET("/events/:id",     handlers.GetEventByIDHandler(pool, eventCache))
		admin.DELETE("/events/:id",  handlers.DeleteEventHandler(pool))
		admin.GET("/events",         handlers.GetAdminEventsHandler(pool))
		admin.GET("/stats",          handlers.GetAdminStatsHandler(pool))
		admin.GET("/merch/oversold", handlers.GetOversoldMerchOrdersHandler(pool))
		// All merch (event-linked + standalone) — for admin dashboard
		admin.GET("/merch",          handlers.GetAllMerchHandler(pool))
		admin.PUT("/merch/:id", handlers.AdminUpdateMerchHandler(pool, eventCache))
		// Create standalone merch (not linked to any event) — admin
		admin.POST("/merch",         handlers.AdminCreateMerchHandler(pool, eventCache))
		admin.GET("/users",          controllers.GetAllUsersHandler(pool))
		admin.PUT("/users/promote/:id",    controllers.PromoteUserHandler(pool))
		admin.PUT("/users/demote/:id",     controllers.DemoteUserHandler(pool))
		admin.DELETE("/users/delete/:id",  handlers.DeleteUserHandler(pool))
		admin.GET("/campaigns",      handlers.AdminGetCampaignsHandler(pool))
	}

	// ==========================================
	// Rate Limit Management (Admin)
	// ==========================================
	if rateLimitManager != nil {
		adminAPI := v1.Group("/admin/ratelimit")
		adminAPI.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
		{
			adminAPI.GET("/ip/:ip",              handlers.RateLimitIPStatusHandler(rateLimitManager))
			adminAPI.POST("/ip/:ip/reset",       handlers.RateLimitIPResetHandler(rateLimitManager))
			adminAPI.GET("/user/:userId",         handlers.RateLimitUserStatusHandler(rateLimitManager))
			adminAPI.POST("/user/:userId/reset",  handlers.RateLimitUserResetHandler(rateLimitManager))
			adminAPI.GET("/metrics",              handlers.RateLimitMetricsHandler())
		}
	}

	// ==========================================
	// Organiser Routes
	// ==========================================
	orgRoutes := v1.Group("/organiser")
	orgRoutes.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	if rateLimitManager != nil {
		orgRoutes.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 50, RefillRate: 2.0, TTL: 15 * time.Minute},
			"organiser-merch"))
	}
	{
		orgRoutes.POST("/events/:id/publish", handlers.PublishEventHandler(pool, eventCache))
		orgRoutes.POST("/events/:id/cancel",  handlers.CancelEventHandler(pool, eventCache))
		orgRoutes.GET("/events",              handlers.GetOrganizerEventsHandler(pool))

		// Merch linked to an event
		orgRoutes.POST("/events/:id/merch",   handlers.OrgCreateMerchHandler(pool, eventCache))
		orgRoutes.PUT("/merch/:id",           handlers.OrgUpdateMerchHandler(pool, eventCache))
		orgRoutes.DELETE("/merch/:id",        handlers.OrgDeactivateMerchHandler(pool, eventCache))
		orgRoutes.POST("/merch/:id/variants", handlers.OrgCreateVariantHandler(pool, eventCache))
		orgRoutes.PUT("/merch/variants/:id",  handlers.OrgUpdateVariantHandler(pool, eventCache))
		// Standalone merch — not linked to any event
		orgRoutes.POST("/merch",              handlers.OrgCreateStandaloneMerchHandler(pool, eventCache))

		// Ticket Types
		orgRoutes.GET("/events/:id/ticket-types",  handlers.OrgGetTicketTypesHandler(pool))
		orgRoutes.POST("/events/:id/ticket-types", handlers.OrgCreateTicketTypesHandler(pool))
		orgRoutes.PUT("/events/:id/ticket-types",  handlers.OrgReplaceTicketTypesHandler(pool))
		orgRoutes.PATCH("/ticket-types/:typeId",   handlers.OrgUpdateTicketTypeHandler(pool))
		orgRoutes.DELETE("/ticket-types/:typeId",  handlers.OrgDeleteTicketTypeHandler(pool))

		orgRoutes.GET("/ad-wallet",             handlers.GetWalletHandler(pool))
		orgRoutes.POST("/ad-wallet/topup",      handlers.TopupWalletHandler(pool, paymentService))
		orgRoutes.POST("/events/:id/campaigns", handlers.CreateCampaignHandler(pool))
		orgRoutes.GET("/campaigns",             handlers.GetMyCampaignsHandler(pool))
		orgRoutes.GET("/campaigns/:id",         handlers.GetCampaignHandler(pool))
		orgRoutes.PATCH("/campaigns/:id",       handlers.UpdateCampaignHandler(pool))
		orgRoutes.DELETE("/campaigns/:id",      handlers.EndCampaignHandler(pool))

		orgRoutes.GET("/dashboard/stats",                handlers.GetOrganizerDashboardStatsHandler(pool))
		orgRoutes.GET("/dashboard/events",               handlers.GetOrganizerDashboardEventsHandler(pool))
		orgRoutes.GET("/dashboard/events/:id/stats",     handlers.GetOrganizerEventStatsHandler(pool))
		orgRoutes.GET("/dashboard/events/:id/bookings",  handlers.GetOrganizerEventBookingsHandler(pool))
		orgRoutes.GET("/dashboard/events/:id/merch",     handlers.GetOrganizerEventMerchHandler(pool))
		orgRoutes.GET("/dashboard/payouts",              handlers.GetOrganizerPayoutSummaryHandler(pool))
	}

	// ==========================================
	// Media Upload Routes
	// ==========================================
	media := v1.Group("/organiser/events/:id/media")
	media.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	media.Use(middleware.ExtendWriteTimeout(5 * time.Minute))
	if rateLimitManager != nil {
		media.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"media-upload"))
	}
	{
		media.POST("/cover",          handlers.UploadCoverImageHandler(pool, imgService))
		media.POST("/banner",         handlers.UploadBannerImageHandler(pool, imgService))
		media.POST("/banner/video",   handlers.UploadBannerVideoHandler(pool, imgService))
		media.DELETE("/banner/video", handlers.DeleteBannerVideoHandler(pool, imgService))
		media.POST("/gallery",        handlers.UploadGalleryImageHandler(pool, imgService))
	}

	// ==========================================
	// Protected Test Route
	// ==========================================
	v1.GET("/protected-test", middleware.AuthMiddleware(cfg), handlers.TestProtectedHandler())

	// ==========================================
	// Start Server — Graceful shutdown
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
		Str("frontend_urls", strings.Join(cfg.FrontendURLs, ", ")).
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