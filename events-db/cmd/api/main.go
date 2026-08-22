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
	"events-db/internal/models"
	"events-db/internal/ratelimit"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"events-db/internal/workers"

	"github.com/gin-contrib/cors"
)

func main() {

	// Register custom binding validators (strong_password, etc.) before any
	// handler is wired up so that binding tags work across the whole app.
	utils.RegisterCustomValidators()

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

	// LOAD_TEST_MODE=true drops every general rate-limit middleware so a
	// single-IP load test (e.g. k6 from one machine) measures real capacity
	// instead of the limiter. The Redis client above is extracted FIRST, so
	// feed/event caching keeps working. Auth endpoints keep their in-memory
	// fail-closed brute-force protection regardless (see rate_limit_auth.go).
	// NEVER leave this enabled in normal production operation.
	if os.Getenv("LOAD_TEST_MODE") == "true" {
		log.Warn().Msg("LOAD_TEST_MODE ENABLED — general rate limiting DISABLED; revert after testing")
		rateLimitManager = nil
	}

	var eventCache *cache.EventCache
	if rdb != nil {
		eventCache = cache.NewEventCache(rdb)
		log.Info().Msg("event cache initialised")
	}

	// ==========================================
	// Push notifications (Expo)
	// ==========================================
	// Constructed BEFORE the workers below, not with the other services further
	// down. The expiry worker runs its first cycle immediately on startup and
	// can write reminder notifications within milliseconds; registering the
	// dispatcher after that would silently swallow the first batch of pushes on
	// every deploy.
	//
	// The dispatcher is a package-level hook in internal/repository rather than
	// a parameter threaded through the workers — see the comment on
	// repository.SetPushDispatcher for why. The upshot: every producer of a
	// notification row gets push delivery automatically, including ones added
	// after this line was written.
	if cfg.PushEnabled {
		pushService := services.NewPushService(pool, cfg)
		pushService.Start(ctx)
		repository.SetPushDispatcher(pushService.Dispatch)
		log.Info().Msg("push notifications enabled")
	} else {
		log.Warn().Msg("PUSH_ENABLED=false — in-app notification feed still active, no device pushes will be sent")
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

	workers.StartReminderWorker(pool, ctx)
	log.Info().Msg("reminder worker started")

	workers.StartSellOutWorker(pool, ctx)
	log.Info().Msg("sell-out worker started")

	// Announces newly published events to the organiser's followers.
	//
	// A worker rather than a hook on the publish path: an event reaches
	// 'published' via AdminApproveEvent, ApproveEventWithFeature AND
	// AdminPromoteEvent, so hooking any one of them would miss the others.
	// Polling for the state instead of the transition covers all three, plus
	// whatever the fourth turns out to be. See the worker for the full note.
	workers.StartOrganizerAnnounceWorker(pool, ctx)
	log.Info().Msg("organiser announce worker started")

	bookingRepo    := repository.NewPgxBookingQuerier(pool)
	eventRepo      := repository.NewPgxEventQuerier(pool)
	cartQuerier    := repository.NewPgxCartQuerier(pool)
	reserveQuerier := repository.NewPgxReservationQuerier(pool)

	// ==========================================
	// Services
	// ==========================================
	emailService   := services.NewEmailService(cfg)
	qrService         := services.NewQRService(pool, cfg)
	rotatingQRService := services.NewRotatingQRService(cfg)
	paymentService := services.NewPaymentService(pool, cfg, bookingJobs)

	// KYC: identity verification for organizers. Uses Didit's DHA registry
	// check when DIDIT_API_KEY is set, manual admin review otherwise.
	// DHA ID check: VerifyNow (SA-local) when its key is set, else Didit, else manual.
	idVerifier := services.NewIDVerifier(cfg.VerifyNowAPIKey, cfg.VerifyNowMode, cfg.DiditAPIKey)
	kycService, err := services.NewKYCService(pool, idVerifier, cfg.KYCEncryptionKey)
	if err != nil {
		log.Fatal().Err(err).Msg("kyc service init failed")
	}
	log.Info().Str("verifier", kycService.VerifierName()).Msg("kyc service ready")

	// Bank-account ownership check runs at payout-detail entry. Optional:
	// without a key, bank details save exactly as before (unverified).
	var verifyNow *services.VerifyNow
	if cfg.VerifyNowAPIKey != "" {
		verifyNow = services.NewVerifyNow(cfg.VerifyNowAPIKey, cfg.VerifyNowMode)
		log.Info().Str("mode", verifyNow.Mode()).Msg("verifynow ready")
	}

	// Two-tier organizer payouts: ledger creation for completed events +
	// Paystack balance transfers for held-tier settlements and reserve releases.
	payoutService := services.NewPayoutService(pool, paymentService.Paystack())
	// Cancellation refunds, clawback accrual, dispute logging, and
	// reconciliation of refunds issued outside the app.
	refundService := services.NewRefundService(pool, paymentService)
	if verifyNow != nil {
		payoutService = payoutService.WithVerifyNow(verifyNow, kycService.Decrypt)
	}
	workers.StartPayoutWorker(pool, payoutService, ctx, 10*time.Minute)

	// Ads/campaigns DISABLED for launch — the ad worker, ad-account, campaign
	// and ad-wallet routes are unregistered below. Feed placement is fully
	// organic. Re-enable by restoring these blocks (handlers/repos still exist).
	// workers.StartAdWorker(pool, ctx)
	imgService := services.NewImageService(cfg)

	// ==========================================
	// Setup Router
	// ==========================================
	router := gin.New()
	// Keep the trusted-proxy list empty (never honour X-Forwarded-For), but
	// take the client IP from Cloudflare's CF-Connecting-IP header.
	//
	// WHY: in production the request path is
	//   client → Cloudflare edge → cloudflared container → api:8080
	// so c.ClientIP() used to resolve to the cloudflared container's docker
	// IP (e.g. 172.18.0.4) for EVERY request. That collapsed all per-IP rate
	// limit buckets (login, password reset, ticket claims…) into ONE global
	// bucket shared by every user on the platform — one abusive client could
	// rate-limit everyone, and audit logs were useless.
	//
	// SAFETY: trusting this header is only sound because the API is not
	// directly reachable — port 8080 binds to 127.0.0.1 on the EC2 host, the
	// security group blocks all inbound traffic, and the Cloudflare Tunnel is
	// the sole ingress. Cloudflare always overwrites CF-Connecting-IP at its
	// edge, so clients can't spoof it. If you EVER expose the API another way
	// (open port, ALB, nginx proxy), revisit this line — a direct caller could
	// then forge the header. In local dev the header is simply absent and Gin
	// falls back to the socket address, so nothing changes there.
	router.SetTrustedProxies(nil)
	router.TrustedPlatform = gin.PlatformCloudflare

	router.Use(middleware.RequestID())
	// Live presence. Registered globally and once: it calls c.Next() first, so
	// user_id is populated by whichever AuthMiddleware ran, whatever group the
	// route belongs to. Requests that never authenticated are skipped.
	router.Use(middleware.TrackPresence(eventCache))
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
	// Service identity (intentionally not versioned — this IS the root)
	// ==========================================
	// Without a handler here, GET / returned Gin's bare 404. URL-categorisation
	// crawlers fetch exactly that path to decide what a host is, and with nothing
	// to read, Symantec's WebPulse rated api.ticket-hawk.com as "Placeholders" —
	// its parked-domain bucket, filed under the Security category group. Networks
	// block that aggressively, and a blocked API fails invisibly in the mobile
	// apps: cached bookings still render while every live call quietly fails.
	// See internal/handlers/root_handler.go for the full reasoning.
	router.GET("/", handlers.ServiceIdentityHandler())
	router.HEAD("/", handlers.ServiceIdentityHandler())
	router.GET("/robots.txt", handlers.RobotsHandler())

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
	// Webhook Routes (intentionally not versioned — Paystack and PayPal call these)
	// ==========================================
	webhooks := router.Group("/webhooks")
	if rateLimitManager != nil {
		webhooks.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 10.0, TTL: 1 * time.Minute},
			"webhooks"))
	}
	{
		webhooks.POST("/paystack", handlers.PaystackWebhookHandler(paymentService, refundService, payoutService))
		webhooks.POST("/paypal",   handlers.PayPalWebhookHandler(paymentService))
	}
	webReturnBase := "https://ticket-hawk.com"
	if len(cfg.FrontendURLs) > 0 && strings.TrimSpace(cfg.FrontendURLs[0]) != "" {
		webReturnBase = strings.TrimSpace(cfg.FrontendURLs[0])
	}
	router.GET("/payments/return", handlers.PaymentReturnHandler(paymentService, webReturnBase))

	// ==========================================
	// API v1
	// ==========================================
	v1 := router.Group("/v1")

	// ==========================================
	// Authentication Routes
	// ==========================================
	// FIX 4: auth endpoints are now ALWAYS rate limited. RateLimitAuthByIP and
	// RateLimitLoginByEmail accept a nil manager and degrade to an in-process
	// token bucket when Redis is down (fail-safe, never open). /login also
	// gets a per-account (email) bucket so one account can't be brute-forced
	// from many rotating IPs.

	// Verifies Google / Apple ID tokens. Constructed once: it owns a JWKS cache
	// so provider signing keys aren't refetched on every sign-in.
	oidcVerifier := services.NewOIDCVerifier(cfg.GoogleClientIDs, cfg.AppleClientIDs)

	auth := v1.Group("/auth")

	// ── /login ── two stacked buckets: per-IP and per-target-account ──────
	loginGroup := auth.Group("")
	loginGroup.Use(
		// 10 attempts per IP, refill 1 token/minute — blocks single-IP brute force
		middleware.RateLimitAuthByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 2 * time.Hour},
			"login"),
		// 5 attempts per account, refill 1 token/2 minutes — blocks
		// distributed brute force against a single email
		middleware.RateLimitLoginByEmail(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 5, RefillRate: 1.0 / 120.0, TTL: 2 * time.Hour},
			"login-email"),
	)
	loginGroup.POST("/login", handlers.LoginHandler(pool, cfg))

	// ── /register ──────────────────────────────────────────────────────────
	registerGroup := auth.Group("")
	registerGroup.Use(middleware.RateLimitAuthByIP(rateLimitManager,
		// 5 attempts, refill 1 token/10 minutes
		ratelimit.BucketConfig{Capacity: 5, RefillRate: 1.0 / 600.0, TTL: 2 * time.Hour},
		"register"))
	registerGroup.POST("/register", handlers.CreateUserhandler(pool))

	// ── social sign-in (Google / Apple) ────────────────────────────────────
	// Rate limited by IP only: there is no target email to key on until the
	// token is verified, and the provider has already done the expensive part.
	// Looser than /login because there is nothing to brute-force here — a
	// forged token fails signature verification, it does not fail "slowly
	// enough to guess". This is abuse protection, not credential protection.
	socialGroup := auth.Group("")
	socialGroup.Use(middleware.RateLimitAuthByIP(rateLimitManager,
		ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 30.0, TTL: 2 * time.Hour},
		"social-auth"))
	socialGroup.POST("/social", handlers.SocialSignInHandler(pool, cfg, oidcVerifier))

	// ── password reset ─────────────────────────────────────────────────────
	passwordResetGroup := auth.Group("")
	passwordResetGroup.Use(middleware.RateLimitAuthByIP(rateLimitManager,
		ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 300.0, TTL: 1 * time.Hour},
		"password-reset"))
	passwordResetGroup.POST("/reset_password_request", handlers.RequestPasswordResetHandler(pool, emailService, cfg))
	passwordResetGroup.POST("/reset_password", handlers.ResetPasswordHandler(pool))

	// ── /refresh ───────────────────────────────────────────────────────────
	refreshGroup := auth.Group("")
	refreshGroup.Use(middleware.RateLimitAuthByIP(rateLimitManager,
		// 20 attempts, refill 1 token/minute
		ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 1 * time.Hour},
		"refresh"))
	refreshGroup.POST("/refresh", handlers.RefreshTokenHandler(pool, cfg))

	// ── /logout ────────────────────────────────────────────────────────────
	auth.POST("/logout", middleware.AuthMiddleware(cfg), handlers.LogoutHandler(pool))

	// Connected-accounts management on the profile screen. Unlinking is guarded
	// server-side: an account with no password cannot remove its last identity,
	// or nothing could ever sign into it again.
	auth.GET("/identities", middleware.AuthMiddleware(cfg), handlers.ListLinkedIdentitiesHandler(pool))
	auth.DELETE("/identities/:provider", middleware.AuthMiddleware(cfg), handlers.UnlinkIdentityHandler(pool))

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
		public.GET("/by-ids",            handlers.GetEventsByIDsHandler(pool))
		public.GET("/search",            handlers.SearchEventsHandler(pool))
		public.GET("/category/:id",      handlers.GetEventCategoryHandler(pool, eventCache))
		public.GET("/filter",            handlers.FilterEventsHandler(pool, eventCache))
		public.GET("/:id/merch",         handlers.GetEventMerchHandler(pool, eventCache))
		public.GET("/:id/merch/preview", handlers.GetEventMerchPreviewHandler(pool, eventCache))
		public.GET("/:id/ticket-types",  handlers.GetTicketTypesHandler(pool))
		public.POST("/:id/view",         handlers.TrackEventViewHandler(pool, eventCache))
		// Retention beacon — how long the viewer had the event page open.
		// Unauthenticated by design (OptionalAuth above means a token is read
		// if present but never required): requiring one would silently drop
		// every logged-out visitor, which is most of the traffic organisers
		// want to measure. Inherits this group's rate limit, and the handler
		// clamps the reported duration and checks the event is publicly
		// visible before writing anything.
		public.POST("/:id/dwell",        handlers.RecordDwellHandler(pool, eventCache))
		// Must be last — catches both UUIDs and slugs
		public.GET("/:id",               handlers.GetEventBySlugHandler(pool, eventCache))
	}

	// ==========================================
	// Public Organiser Profile Routes
	//
	// GET /organisers/:slug         — profile (accepts slug or UUID)
	// GET /organisers/:slug/events  — ?scope=upcoming|past
	//
	// OptionalAuth so a signed-in visitor is identified when a token is
	// present. Nothing here needs it yet; phase 2's follow state will, and
	// adding the middleware later would silently change behaviour on an
	// endpoint the clients cache.
	//
	// An organiser that cannot be resolved 404s identically whether they do
	// not exist, are suspended, or have hidden their page — see
	// repository.ErrOrganizerProfileNotFound for why the causes are not
	// distinguished.
	//
	// Its own rate-limit bucket rather than sharing "events-public": a visitor
	// paging through an organiser's back catalogue burns requests fast, and
	// sharing the bucket would throttle them out of the main feed.
	// ==========================================
	organisersPublic := v1.Group("/organisers")
	organisersPublic.Use(middleware.OptionalAuth(cfg))
	if rateLimitManager != nil {
		organisersPublic.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 120, RefillRate: 3.0, TTL: 15 * time.Minute},
			"organisers-public"))
	}
	{
		organisersPublic.GET("/:slug",        handlers.GetPublicOrganizerHandler(pool))
		organisersPublic.GET("/:slug/events", handlers.GetPublicOrganizerEventsHandler(pool))
	}

	// American-spelling alias for the same two routes.
	//
	// This codebase is already inconsistent — admin routes say /organizers, the
	// organiser group says /organiser — and these are PUBLIC URLs that end up
	// in shared links and search results. A 404 on a plausible spelling is a
	// support ticket, not a principle.
	//
	// Registered as a second group because gin cannot hold two different
	// wildcard names at the same position in one tree; a separate static prefix
	// gets its own subtree and sidesteps that entirely.
	organizersPublicAlias := v1.Group("/organizers")
	organizersPublicAlias.Use(middleware.OptionalAuth(cfg))
	if rateLimitManager != nil {
		organizersPublicAlias.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 120, RefillRate: 3.0, TTL: 15 * time.Minute},
			"organisers-public"))
	}
	{
		organizersPublicAlias.GET("/:slug",        handlers.GetPublicOrganizerHandler(pool))
		organizersPublicAlias.GET("/:slug/events", handlers.GetPublicOrganizerEventsHandler(pool))
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
	// Public Collection Routes
	// ==========================================
	collectionsPublic := v1.Group("/collections")
	collectionsPublic.Use(middleware.OptionalAuth(cfg))
	if rateLimitManager != nil {
		collectionsPublic.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 5.0, TTL: 15 * time.Minute},
			"collections"))
	}
	{
		collectionsPublic.GET("/:id", handlers.GetCollectionByIDHandler(pool))
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
			// Guests poll GET /guest/bookings/:bn/payment every ~2.5s during
			// checkout, so this bucket must support sustained polling (matches
			// the authed "payments" bucket) rather than the old 10/1-per-minute.
			ratelimit.BucketConfig{Capacity: 150, RefillRate: 3.0, TTL: 15 * time.Minute},
			"guest-bookings"))
	}

	// Brute-force / enumeration guard for every bookingNumber + email route.
	//
	// Replaces the per-handler lockout that only covered two endpoints while
	// /payment, /refund-quote, /offline, /pool-qr and POST /refund accepted the
	// same credential with none. Keyed on booking+email AND on IP, so a
	// stranger guessing an address cannot lock the real owner out of their own
	// ticket. Safe with a nil client: returns a pass-through handler and logs a
	// warning, which is what happens when Redis is down at startup.
	guest.Use(middleware.GuestBookingGuard(rdb))
	{
		guest.POST("/bookings",                              handlers.CreateGuestBookingHandler(pool, paymentService))
		guest.GET("/bookings/:bookingNumber",                handlers.GetGuestBookingHandler(pool))
		guest.GET("/bookings/:bookingNumber/assignments",    handlers.GetGuestTicketAssignmentsHandler(pool))
		guest.GET("/bookings/:bookingNumber/assignments/:assignmentId/qr", handlers.GetGuestRotatingQRTokenHandler(pool, rotatingQRService))
		guest.GET("/bookings/:bookingNumber/pool-qr",                      handlers.GetGuestPoolQRTokenHandler(pool, rotatingQRService))
		guest.GET("/bookings/:bookingNumber/offline",                     handlers.GetGuestOfflineTicketsHandler(pool, rotatingQRService))
		guest.GET("/bookings/:bookingNumber/payment",        handlers.GuestPaymentStatusHandler(pool, paymentService))
		guest.POST("/bookings/:bookingNumber/payment/start", handlers.GuestStartPaymentHandler(pool, paymentService))
		guest.POST("/bookings/:bookingNumber/payment/retry", handlers.GuestRetryPaymentHandler(pool, paymentService))
		guest.GET("/bookings/:bookingNumber/refund-quote",   handlers.GuestRefundQuoteHandler(pool))
		guest.POST("/bookings/:bookingNumber/refund",        handlers.GuestRefundHandler(pool, paymentService, emailService))
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
		notifs.GET("",            handlers.GetNotificationsHandler(pool))
		notifs.GET("/unread",     handlers.GetUnreadCountHandler(pool))
		notifs.PATCH("/:id/read", handlers.MarkReadHandler(pool))
		notifs.POST("/read-all",  handlers.MarkAllReadHandler(pool))

		// Device registry for out-of-app push. The app registers on every cold
		// start (Expo rotates tokens without warning) and unregisters on logout
		// and when the Settings toggle goes off.
		notifs.POST("/push-token",   handlers.RegisterPushTokenHandler(pool))
		notifs.DELETE("/push-token", handlers.UnregisterPushTokenHandler(pool))
		// Sends a real push to the caller's own devices through the real
		// pipeline. Safe in production: authed, rate-limited, self-targeted.
		notifs.POST("/push-test",    handlers.SendTestPushHandler(pool))
	}

	// ==========================================
	// Bookings Routes
	// ==========================================
	bookings := v1.Group("/bookings")
	bookings.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		bookings.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 120, RefillRate: 5.0, TTL: 15 * time.Minute},
			"bookings"))
	}
	{
		bookings.POST("",    handlers.CreateBookingHandler(bookingRepo, eventRepo, paymentService, cfg))
		bookings.GET("",     handlers.GetUserBookingsHandler(bookingRepo))
		bookings.GET("/:id", handlers.GetBookingDetailsHandler(bookingRepo))
		bookings.POST("/:id/refund",                      handlers.RequestRefundHandler(pool, bookingRepo, eventRepo, paymentService, emailService))
		bookings.GET("/:id/refund-quote",                 handlers.RefundQuoteHandler(bookingRepo, eventRepo))
		bookings.GET("/:id/assignments",                  handlers.GetTicketAssignmentsHandler(pool))
		bookings.GET("/:id/pool-qr",                      handlers.GetPoolQRTokenHandler(pool, rotatingQRService))
		bookings.GET("/:id/offline",                      handlers.GetOfflineTicketsHandler(pool, rotatingQRService))
		bookings.POST("/:id/assignments/:assignmentId",   handlers.AssignTicketHandler(pool, emailService, cfg.FrontendURLs[0]))
		bookings.DELETE("/:id/assignments/:assignmentId", handlers.RevokeAssignmentHandler(pool))
		bookings.POST("/:id/verify",                      handlers.VerifyQRCodeHandler(bookingRepo, eventRepo, qrService))

		if rateLimitManager != nil {
			paymentRoutes := bookings.Group("")
			paymentRoutes.Use(middleware.RateLimitByUser(rateLimitManager,
				ratelimit.BucketConfig{Capacity: 150, RefillRate: 3.0, TTL: 15 * time.Minute},
				"payments"))
			paymentRoutes.POST("/:id/payment",       handlers.CreatePaymentHandler(pool, paymentService))
			paymentRoutes.GET("/:id/payment",        handlers.GetPaymentStatusHandler(pool, paymentService))
			paymentRoutes.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentService))
		} else {
			bookings.POST("/:id/payment",       handlers.CreatePaymentHandler(pool, paymentService))
			bookings.GET("/:id/payment",        handlers.GetPaymentStatusHandler(pool, paymentService))
			bookings.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentService))
		}
	}

	// ==========================================
	// Ticket Routes
	// ==========================================
	tickets := v1.Group("/tickets")
	{
		// Public: claim an assigned ticket (no auth)
		publicTickets := tickets.Group("")
		if rateLimitManager != nil {
			publicTickets.Use(middleware.RateLimitByIP(rateLimitManager,
				ratelimit.BucketConfig{Capacity: 10, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
				"ticket-claim"))
		}
		publicTickets.POST("/claim", handlers.ClaimTicketHandler(pool, emailService, cfg.FrontendURLs[0]))

		// Public: live rotating QR for a CLAIMED slot, authed by
		// assignment UUID + claimant email (mirrors the guest QR endpoint).
		// Powers the /claim-ticket "my ticket" page linked from the claim
		// confirmation email.
		//
		// Separate bucket from /claim: the ticket page polls this every
		// ~28s (and many attendees share carrier NAT at the venue), so the
		// tight 10-cap/1-per-min claim bucket would starve it. Matches the
		// guest-bookings polling bucket instead.
		claimedQR := tickets.Group("")
		if rateLimitManager != nil {
			claimedQR.Use(middleware.RateLimitByIP(rateLimitManager,
				ratelimit.BucketConfig{Capacity: 150, RefillRate: 3.0, TTL: 15 * time.Minute},
				"claimed-ticket-qr"))
		}
		claimedQR.GET("/claimed/:assignmentId/qr", handlers.GetClaimedRotatingQRTokenHandler(pool, rotatingQRService))

		// Authenticated: rotating QR token (ticket holder) + organiser scan
		authTickets := tickets.Group("")
		authTickets.Use(middleware.AuthMiddleware(cfg))
		if rateLimitManager != nil {
			authTickets.Use(middleware.RateLimitByUser(rateLimitManager,
				ratelimit.BucketConfig{Capacity: 120, RefillRate: 2.0, TTL: 5 * time.Minute},
				"qr-token"))
		}
		authTickets.GET("/:assignmentId/qr",   handlers.GetRotatingQRTokenHandler(pool, rotatingQRService))
		authTickets.POST("/:assignmentId/scan", handlers.ScanRotatingQRHandler(pool, rotatingQRService))
		authTickets.POST("/scan",               handlers.ScanHandler(pool, rotatingQRService))
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
		// PATCH, not PUT: the edit-profile form submits only changed fields, and
		// a sparse patch means an omitted field is left alone rather than cleared.
		//
		// Takes oidcVerifier because changing the EMAIL address requires step-up
		// re-authentication, and a Google/Apple account has no password to check.
		// It proves identity with a fresh provider token instead — see
		// handlers/reauth.go.
		user.PATCH("/me",         handlers.UpdateMyProfileHandler(pool, emailService, oidcVerifier))
		// Account deletion (Apple Guideline 5.1.1(v)). The preview drives the
		// confirmation screen; the DELETE re-authenticates and anonymises.
		//
		// Same reason for oidcVerifier: without it a social-only account could
		// never satisfy the password check and therefore could never delete
		// itself, which is the exact thing 5.1.1(v) requires.
		user.GET("/me/deletion-preview", handlers.GetAccountDeletionPreviewHandler(pool))
		user.DELETE("/me",               handlers.DeleteMyAccountHandler(pool, oidcVerifier))
		user.GET("/merch-orders", handlers.GetMyMerchOrdersHandler(pool))
		user.GET("/following",    handlers.GetFollowedEventsHandler(pool))
		// Organisers, not events. Separate endpoint rather than a ?type= on
		// /following because the two return different shapes — an event card
		// versus an organiser card — and one endpoint returning either is a
		// discriminated union the clients would have to unpick.
		user.GET("/following-organisers", handlers.GetFollowedOrganizersHandler(pool))
	}

	// ==========================================
	// Categories
	// ==========================================
	categoriesGroup := v1.Group("/categories")
	if rateLimitManager != nil {
		categoriesGroup.Use(middleware.RateLimitByIP(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 1.0, TTL: 15 * time.Minute},
			"categories"))
	}
	{
		categoriesGroup.GET("", handlers.GetCategoriesHandler(pool))
	}

	// ==========================================
	// Event Follow Routes (authenticated)
	// POST /events/:id/follow  — toggle follow on/off
	// GET  /events/:id/follow  — check follow status + count
	// GET  /user/following     — registered above under the user group
	// ==========================================
	eventFollowGroup := v1.Group("/events")
	eventFollowGroup.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		eventFollowGroup.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"event-follow"))
	}
	{
		eventFollowGroup.POST("/:id/follow", handlers.ToggleFollowHandler(pool))
		eventFollowGroup.GET("/:id/follow",  handlers.GetFollowStatusHandler(pool))
	}

	// ==========================================
	// Organiser Follow Routes (authenticated)
	// POST /organisers/:slug/follow  — toggle follow on/off
	// GET  /organisers/:slug/follow  — status + follower count
	// GET  /user/following-organisers — registered under the user group below
	//
	// A separate group from organisersPublic for the same reason
	// eventFollowGroup is separate from the public events group: that group
	// runs OptionalAuth, and a follow needs a real user rather than one that
	// might be there. Gin merges the two into one tree, so /organisers/:slug
	// stays public while /organisers/:slug/follow requires a token.
	//
	// The slug resolves through the same public gate as the profile, so a
	// suspended or unverified organiser cannot accumulate followers while
	// invisible.
	// ==========================================
	organiserFollowGroup := v1.Group("/organisers")
	organiserFollowGroup.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		organiserFollowGroup.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 60, RefillRate: 2.0, TTL: 15 * time.Minute},
			"organiser-follow"))
	}
	{
		organiserFollowGroup.POST("/:slug/follow", handlers.ToggleOrganizerFollowHandler(pool))
		organiserFollowGroup.GET("/:slug/follow",  handlers.GetOrganizerFollowStatusHandler(pool))
	}

	// ==========================================
	// Event Staffing (scanner assignment & join codes)
	// ==========================================
	eventStaff := v1.Group("/events")
	eventStaff.Use(middleware.AuthMiddleware(cfg))
	{
		eventStaff.GET("/:id/staff",                  handlers.ListEventStaffHandler(pool))
		eventStaff.POST("/:id/staff",                 handlers.AddEventStaffHandler(pool))
		eventStaff.DELETE("/:id/staff/:userId",       handlers.RevokeEventStaffHandler(pool))
		eventStaff.POST("/:id/staff/codes",           handlers.CreateStaffCodeHandler(pool))
		eventStaff.GET("/:id/staff/codes",            handlers.ListStaffCodesHandler(pool))
		eventStaff.DELETE("/:id/staff/codes/:codeId", handlers.RevokeStaffCodeHandler(pool))
	}

	// Staffer-side: redeem a join code / list events I can scan. The redeem
	// endpoint gets a strict per-user bucket — it is the only endpoint where
	// guessing is the attack (40-bit codes + this limit = not practical).
	staffSelf := v1.Group("/staff")
	staffSelf.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		staffSelf.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 10, RefillRate: 0.2, TTL: 15 * time.Minute},
			"staff-redeem"))
	}
	{
		staffSelf.POST("/redeem", handlers.RedeemStaffCodeHandler(pool))
		staffSelf.GET("/events",  handlers.ListMyScanEventsHandler(pool))
	}

	// ==========================================
	// Offline scanning: credential, manifest, scan-event sync
	// ==========================================
	scannerCredService := services.NewScannerCredentialService(cfg)
	scanSync := v1.Group("/staff/events")
	scanSync.Use(middleware.AuthMiddleware(cfg))
	if rateLimitManager != nil {
		scanSync.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 240, RefillRate: 4.0, TTL: 10 * time.Minute},
			"scan-sync"))
	}
	{
		scanSync.POST("/:id/scanner-credential", handlers.MintScannerCredentialHandler(pool, scannerCredService))
		scanSync.GET("/:id/manifest",            handlers.GetManifestHandler(pool))
		scanSync.POST("/:id/scans",              handlers.IngestScansHandler(pool))
	}
	eventStaff.GET("/:id/scan-exceptions", handlers.ScanExceptionsHandler(pool))

	// ==========================================
	// Home Feed
	// ==========================================
	v1.GET("/home", middleware.OptionalAuth(cfg), handlers.HomeSectionsHandler(pool, eventCache))

	// ==========================================
	// Drops Feed
	// ==========================================
	v1.GET("/drops", handlers.GetDropsFeedHandler(pool))

	// ==========================================
	// Staff invite acceptance — token-authenticated, no session yet.
	v1.GET("/staff/invite",           handlers.GetStaffInviteHandler(pool))
	// The code goes to the master admins, never to the invitee.
	v1.POST("/staff/invite/start", handlers.StartStaffInviteHandler(pool,
		func(adminEmail, code, invitee string) error {
			return emailService.SendStaffInviteCode(adminEmail, code, invitee)
		}))
	v1.POST("/staff/invite/complete", handlers.CompleteStaffInviteHandler(pool))

	// Admin Routes
	// ==========================================
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	// Invited staff hold role='admin' so they can sign in; staff_role is what
	// limits them. Without this, a read_only invitee could call any legacy
	// admin write endpoint directly. Accounts with no staff_role are untouched.
	admin.Use(middleware.RequireStaffWrite(pool))
	if rateLimitManager != nil {
		admin.Use(middleware.CompositeRateLimit(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 5.0, TTL: 1 * time.Hour},
			ratelimit.BucketConfig{Capacity: 200, RefillRate: 10.0, TTL: 30 * time.Minute},
			"admin"))
	}
	{
		// ── Staff capability layer (migration 000068) ─────────────────────
		//
		// Registered INSIDE the existing admin group, so these inherit
		// AuthMiddleware + AdminOnlyMiddleware + the admin rate limit, and add
		// RequireStaff on top.
		//
		// DELIBERATELY NOT `admin.Use(middleware.RequireStaff(pool))`.
		//
		// Migration 000068 leaves users.staff_role NULL for every existing
		// account — by design, so console access is granted rather than
		// inherited. Putting RequireStaff on the whole group would therefore
		// 403 every current admin the moment this deploys, including whoever
		// needs to log in and grant the first staff_role. The console would
		// lock its own key inside.
		//
		// So the new endpoints are gated now, the existing ones are not, and
		// the group-level switch is flipped in Phase 3 once staff roles have
		// actually been assigned. See the bootstrap note in the README.
		staffAPI := admin.Group("")
		staffAPI.Use(middleware.RequireStaff(pool))
		{
			// What the console may draw. No capability required beyond being
			// staff — it is the call that TELLS you what you have.
			staffAPI.GET("/me/capabilities", handlers.GetMyCapabilitiesHandler(pool))

			// Layer 1 — log a request. Editors and admins.
			staffAPI.POST("/requests",
				middleware.RequireCapability(pool, models.CapRequestDangerous),
				handlers.CreateStaffRequestHandler(pool))
			staffAPI.GET("/requests",
				middleware.RequireCapability(pool, models.CapRequestDangerous),
				handlers.ListStaffRequestsHandler(pool))

			// Layer 2 — a DIFFERENT admin approves. The four-eyes rule is
			// enforced by the handler's WHERE clause and, independently, by a
			// CHECK constraint in 000068.
			staffAPI.POST("/requests/:id/approve",
				middleware.RequireCapability(pool, models.CapApproveRequests),
				handlers.ApproveStaffRequestHandler(pool))
			staffAPI.POST("/requests/:id/reject",
				middleware.RequireCapability(pool, models.CapApproveRequests),
				handlers.RejectStaffRequestHandler(pool))

			// Layer 3 — the affected organiser confirms by OTP. Sending the
			// code needs the requester's capability; the code itself goes to
			// the organiser and is never returned in the response.
			staffAPI.POST("/requests/:id/send-code",
				middleware.RequireCapability(pool, models.CapRequestDangerous),
				handlers.SendConfirmationCodeHandler(pool,
					func(to, code string) error {
						return emailService.SendStaffConfirmationCode(to, code)
					}))
			staffAPI.POST("/requests/:id/confirm",
				handlers.ConfirmStaffRequestHandler(pool))

			// ── Phase 3: capability-scoped people listings ─────────────────
			//
			// Replaces the console's use of /admin/users, which returned every
			// field to every admin. This endpoint changes the SHAPE of its
			// response by capability — a read_only caller's payload has no
			// email key at all, so there is nothing for the client to leak.
			//
			// The old /admin/users route is left registered: other tooling may
			// call it, and removing it is a separate decision from adding this.
			staffAPI.GET("/people",
				middleware.RequireCapability(pool, models.CapViewUserList),
				handlers.GetScopedPeopleHandler(pool))

			// Granting console access. Admin only, and the handler additionally
			// refuses self-changes and removal of the last admin — either would
			// lock everyone out with no recovery path short of psql.
			// Staff invitations. Accepting needs the emailed link AND an OTP
			// that goes to the master admin, who passes it on out of band.
			staffAPI.POST("/staff/invites",
				middleware.RequireCapability(pool, models.CapManageStaff),
				handlers.CreateStaffInviteHandler(pool,
					func(to, link, role string) error {
						return emailService.SendStaffInvite(to, link, role)
					}))
			staffAPI.GET("/staff/invites",
				middleware.RequireCapability(pool, models.CapManageStaff),
				handlers.ListStaffInvitesHandler(pool))
			staffAPI.DELETE("/staff/invites/:id",
				middleware.RequireCapability(pool, models.CapManageStaff),
				handlers.RevokeStaffInviteHandler(pool))

			staffAPI.PUT("/staff/:id/role",
				middleware.RequireCapability(pool, models.CapManageStaff),
				handlers.SetStaffRoleHandler(pool))

			// ── Phase 4: GMV and the dashboard lists ───────────────────────
			//
			// Separate from /admin/stats, which reports SUM(subtotal) as
			// "revenue". That figure is ticket FACE VALUE — almost all of it is
			// the organiser's money, and reporting it as revenue overstates
			// what the business earns by roughly the inverse of the take rate.
			// This endpoint splits volume (GMV) from what TicketHawk keeps.
			//
			// Gated on CapViewUserList — the weakest staff capability — because
			// these are platform aggregates containing no personal data.
			staffAPI.GET("/stats/gmv",
				middleware.RequireCapability(pool, models.CapViewUserList),
				handlers.GetPlatformGMVHandler(pool))
			staffAPI.GET("/dashboard/lists",
				middleware.RequireCapability(pool, models.CapViewUserList),
				handlers.GetDashboardListsHandler(pool))

			// ── Phase 5: platform analytics time series ────────────────────
			// Aggregates only — no personal data — so the weakest staff
			// capability gates it, same as the GMV endpoint above.
			// Polled every 60s by the console, so it is deliberately Redis-only
			// rather than another field on the GMV aggregate.
			staffAPI.GET("/stats/presence",
				middleware.RequireCapability(pool, models.CapViewUserList),
				handlers.GetPresenceHandler(eventCache))

			staffAPI.GET("/analytics/series",
				middleware.RequireCapability(pool, models.CapViewUserList),
				handlers.GetAdminAnalyticsSeriesHandler(pool))

			// ── Phase 6: event review and approval-with-featuring ──────────
			//
			// The review endpoint returns the FULL event plus its ticket tiers.
			// The old approvals screen showed a summary, which meant nobody
			// ever saw the tiers — where the actual mistakes are (a VIP tier
			// priced at R0, unlimited capacity at a 200-seat venue).
			// event.view, NOT event.approve.
			//
			// This is what the eye icon on the events list opens. Gating it on
			// event.approve meant read_only and access — the two roles whose
			// entire job is looking at things — clicked "view details" and got a
			// 403. Every staff level can now open an event; only the roles that
			// hold event.approve see the approve/reject controls on it.
			staffAPI.GET("/events/:id/review",
				middleware.RequireCapability(pool, models.CapViewEvents),
				handlers.GetEventReviewHandler(pool))

			// Approve, optionally with a BOUNDED featuring window. Replaces the
			// console's "Approve & Add to Home", which set is_featured with no
			// end date — which is how a finished event stays on the homepage.
			staffAPI.PUT("/events/:id/approve-with-feature",
				middleware.RequireCapability(pool, models.CapFeatureEvent),
				handlers.ApproveEventWithFeatureHandler(pool, eventCache))

			// ── Phase 7: execution, the last gate ──────────────────────────
			//
			// Everything before this was ABOUT an action; this is where one
			// happens. The handler does NOT re-decide authorisation — four-eyes,
			// subject confirmation and cooling-off were settled earlier, and the
			// staff_requests_execution_gated CHECK refuses the write if any were
			// skipped. It takes a row lock so two admins clicking Execute on the
			// same queued request cannot both proceed.
			staffAPI.POST("/requests/:id/execute",
				middleware.RequireCapability(pool, models.CapApproveRequests),
				handlers.ExecuteStaffRequestHandler(pool, eventCache))

			// Editors log a takedown; an admin approves it; only then does it
			// run. The event is UNPUBLISHED, not cancelled or deleted — ticket
			// holders keep their tickets and their QR codes keep scanning.
			staffAPI.POST("/events/:id/request-takedown",
				middleware.RequireCapability(pool, models.CapRequestDangerous),
				handlers.RequestEventTakedownHandler(pool))
		}

		// ── Identity documents — MASTER ADMIN ONLY ─────────────────────────
		//
		// Registered OUTSIDE staffAPI on purpose. These are not gated by a
		// console capability at all: no value of console_role reaches them.
		// RequireMasterAdmin reads users.is_master_admin, a separate column, so
		// growing the admin team never grants document access.
		//
		// Every read requires a stated reason and is written to
		// kyc_document_access_log BEFORE the bytes are returned — if the log
		// write fails, the read is refused.
		kycDocs := admin.Group("/kyc")
		{
			// Two-phase, decided by the ORGANISER'S state rather than the
			// caller's role:
			//   pending  → any staff role may look, because you cannot verify
			//              an identity you are not allowed to see
			//   verified → master admin only, because the document has done its
			//              job and later reads are regulatory, not routine
			kycDocs.POST("/:id/document",
				handlers.RequireDocumentAccess(pool),
				handlers.GetIdentityDocumentHandler(pool, "document"))

			// Proof of address goes through the SAME gate. It used to have an
			// ungated GET beside the ID photo's, which meant the weaker of the
			// two documents was the way in. One rule, both documents.
			kycDocs.POST("/:id/proof-of-address",
				handlers.RequireDocumentAccess(pool),
				handlers.GetIdentityDocumentHandler(pool, "proof-of-address"))

			// The log itself stays master-admin only: it names who looked at
			// whose identity document, which is more sensitive than any single
			// entry in it.
			kycDocs.GET("/access-log",
				handlers.RequireMasterAdmin(pool),
				handlers.GetDocumentAccessLogHandler(pool))

			// Editors and admins ask; a master admin approves. Each grant is
			// scoped to one organiser and expires, so it never becomes standing
			// access and revoking it does not mean demoting anyone.
			// Only editor and admin can HOLD a grant, so only they may ask for
			// one. Letting read_only file a request that can never be approved
			// just fills the master admin's queue with noise.
			kycDocs.POST("/:id/request-access",
				middleware.RequireCapabilityOn(pool, models.CapRequestDangerous),
				handlers.RequestDocumentAccessHandler(pool))
			kycDocs.GET("/grants",
				handlers.RequireMasterAdmin(pool),
				handlers.ListDocumentGrantsHandler(pool))
			kycDocs.POST("/grants/:id/approve",
				handlers.RequireMasterAdmin(pool),
				handlers.ApproveDocumentGrantHandler(pool))
			kycDocs.POST("/grants/:id/deny",
				handlers.RequireMasterAdmin(pool),
				handlers.DenyDocumentGrantHandler(pool))
		}

		// ── Legacy admin endpoints, now capability-gated ──────────────────
		//
		// These predate the staff model. RequireStaffWrite gates their WRITES at
		// one coarse level (CapApproveEvent) and their READS not at all, which
		// gets two things wrong at once: an `access` invitee could GET every
		// organiser's payout profile, and an `editor` could delete a user account.
		//
		// RequireCapabilityOn gives each route a real capability while keeping the
		// property that made RequireStaffWrite safe to deploy: an account with NO
		// staff_role is a pre-existing platform admin and passes through
		// unchanged. See the middleware for why that branch has to exist.
		admin.POST("/events",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.CreateEventHandler(pool, eventCache, emailService, cfg))
		// Moving an event to a different organiser moves its MONEY. Admin only.
		admin.PUT("/events/:id/organizer",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			handlers.AdminTransferEventHandler(pool, eventCache, emailService, cfg))
		// CapEditEvents is now held by admin only. An editor changes a live event
		// by filing an event_edit request carrying the field diff, which an admin
		// approves and executes — see models.ReqEventEdit.
		admin.PUT("/events/:id",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminUpdateEventHandler(pool, eventCache))
		admin.GET("/events/:id",
			middleware.RequireCapabilityOn(pool, models.CapViewEvents),
			handlers.AdminGetEventByIDHandler(pool, eventCache))
		// Deleting an event is not recoverable and strands ticket holders. It is
		// the action a takedown REQUEST exists to avoid, so it sits at admin.
		admin.DELETE("/events/:id",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			handlers.DeleteEventHandler(pool, eventCache))
		// Unscoped cancel. The organiser route filters on organizer_id, so an
		// admin cancelling another organiser's event there matches no rows.
		// Cancelling triggers refunds, so it is admin-level too.
		admin.POST("/events/:id/cancel",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			handlers.AdminCancelEventHandler(pool, eventCache, refundService))
		admin.GET("/events",
			middleware.RequireCapabilityOn(pool, models.CapViewEvents),
			handlers.GetAdminEventsHandler(pool))
		admin.GET("/stats",
			middleware.RequireCapabilityOn(pool, models.CapViewUserList),
			handlers.GetAdminStatsHandler(pool))
		admin.GET("/merch/oversold",
			middleware.RequireCapabilityOn(pool, models.CapViewEvents),
			handlers.GetOversoldMerchOrdersHandler(pool))
		admin.GET("/merch",
			middleware.RequireCapabilityOn(pool, models.CapViewEvents),
			handlers.GetAllMerchHandler(pool))
		admin.PUT("/merch/:id",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminUpdateMerchHandler(pool, eventCache))
		admin.POST("/merch",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminCreateMerchHandler(pool, eventCache))
		admin.POST("/events/:id/merch",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminCreateEventMerchHandler(pool, eventCache))
		admin.PUT("/merch/variants/:id",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminUpdateVariantHandler(pool))
		admin.POST("/upload",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.UploadAdminImageHandler(imgService))
		admin.PUT("/merch/:id/collection",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminAssignMerchToCollectionHandler(pool))
		admin.POST("/collections",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminCreateCollectionHandler(pool))
		admin.GET("/collections",
			middleware.RequireCapabilityOn(pool, models.CapViewEvents),
			handlers.AdminGetCollectionsHandler(pool))
		admin.PUT("/collections/:id",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminUpdateCollectionHandler(pool))
		admin.DELETE("/collections/:id",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.AdminDeactivateCollectionHandler(pool))
		admin.POST("/collections/:id/image",
			middleware.RequireCapabilityOn(pool, models.CapEditEvents),
			handlers.UploadCollectionCoverImageHandler(pool, imgService, true))
		// /admin/users returns every field to every caller and is superseded by
		// /admin/people, which shapes its response by capability. Left registered
		// for other tooling, but no longer readable by staff who cannot see
		// contact details — which is the whole reason /admin/people exists.
		admin.GET("/users",
			middleware.RequireCapabilityOn(pool, models.CapViewUserContact),
			controllers.GetAllUsersHandler(pool))
		// Changing what an account can do on the platform is a four-eyes action
		// (ReqRoleChange). Admin only.
		admin.PUT("/users/promote/:id",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			controllers.PromoteUserHandler(pool))
		admin.PUT("/users/demote/:id",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			controllers.DemoteUserHandler(pool))
		admin.DELETE("/users/delete/:id",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			handlers.DeleteUserHandler(pool))
		// admin.GET("/campaigns", handlers.AdminGetCampaignsHandler(pool)) // ads disabled for launch

		// Event approval queue
		admin.GET("/events/pending",
			middleware.RequireCapabilityOn(pool, models.CapViewEvents),
			handlers.AdminGetPendingEventsHandler(pool))
		admin.PUT("/events/:id/approve",
			middleware.RequireCapabilityOn(pool, models.CapApproveEvent),
			handlers.AdminApproveEventHandler(pool, eventCache))
		admin.PUT("/events/:id/reject",
			middleware.RequireCapabilityOn(pool, models.CapApproveEvent),
			handlers.AdminRejectEventHandler(pool))
		admin.PUT("/events/:id/promote",
			middleware.RequireCapabilityOn(pool, models.CapFeatureEvent),
			handlers.AdminPromoteEventHandler(pool, eventCache))
		admin.PUT("/events/:id/unpromote",
			middleware.RequireCapabilityOn(pool, models.CapFeatureEvent),
			handlers.AdminUnpromoteEventHandler(pool, eventCache))

		// Merch approval queue
		admin.GET("/merch/pending",
			middleware.RequireCapabilityOn(pool, models.CapViewEvents),
			handlers.AdminGetPendingMerchHandler(pool))
		admin.PUT("/merch/:id/approve",
			middleware.RequireCapabilityOn(pool, models.CapApproveEvent),
			handlers.AdminApproveMerchHandler(pool))
		admin.PUT("/merch/:id/reject",
			middleware.RequireCapabilityOn(pool, models.CapApproveEvent),
			handlers.AdminRejectMerchHandler(pool))

		// Organiser account management
		admin.GET("/organizers",
			middleware.RequireCapabilityOn(pool, models.CapViewUserContact),
			handlers.AdminGetOrganizersHandler(pool))
		admin.PUT("/organizers/:id/status",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			handlers.AdminUpdateOrganizerStatusHandler(pool))

		// KYC review queue
		//
		// ⚠️  THE TWO GET DOCUMENT ROUTES THAT USED TO LIVE HERE ARE DELETED.
		//
		//	admin.GET("/kyc/:id/document")
		//	admin.GET("/kyc/:id/proof-of-address")
		//
		// They streamed raw ID photographs to any caller holding users.role =
		// 'admin', with no capability check, no reason, and no entry in
		// kyc_document_access_log. RequireStaffWrite did not touch them because it
		// only gates mutating verbs, so a read_only invitee could fetch any
		// organiser's identity document with one GET — at any point in that
		// organiser's life, years after verification.
		//
		// That route was the back door around every other control in this file:
		// RequireDocumentAccess, the master-admin lock, the grant system and the
		// access log were all in place and all bypassed, because the console
		// fetched the bytes from here instead.
		//
		// Documents now leave through POST /admin/kyc/:id/document only, registered
		// above behind RequireDocumentAccess.
		admin.GET("/kyc",
			middleware.RequireCapabilityOn(pool, models.CapViewKYCStatus),
			handlers.AdminListKYCHandler(pool))
		// The detail response carries the DECRYPTED SA ID number, so it needs the
		// KYC capability rather than sitting open to any staff role.
		admin.GET("/kyc/:id",
			middleware.RequireCapabilityOn(pool, models.CapViewKYCStatus),
			handlers.AdminGetKYCDetailHandler(pool, kycService))
		admin.POST("/kyc/:id/decision",
			middleware.RequireCapabilityOn(pool, models.CapVerifyKYC),
			handlers.AdminDecideKYCHandler(pool))
		admin.POST("/kyc/:id/registry-check",
			middleware.RequireCapabilityOn(pool, models.CapVerifyKYC),
			handlers.AdminRunRegistryCheckHandler(kycService))

		// ── Payouts ───────────────────────────────────────────────────────
		//
		// The payout profile list carries bank account names and tails, so it
		// needs bank.view_tail rather than being open to any staff role.
		admin.GET("/organizers/payouts",
			middleware.RequireCapabilityOn(pool, models.CapViewBankTail),
			handlers.AdminListOrganizerPayoutProfilesHandler(pool))
		// Tier decides whether money settles instantly or is held. Admin only.
		admin.PUT("/organizers/:id/tier",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			handlers.AdminSetOrganizerTierHandler(pool, payoutService))
		// Writing payout bank details is THE most dangerous action in the product
		// — it is how ticket money gets redirected, and it is why ReqBankAdd
		// carries subject confirmation, a 60-minute cooling-off and a 48h payout
		// hold.
		//
		// This direct route bypasses every one of those, so it is now MASTER ADMIN
		// ONLY. An admin who needs a bank change files a bank_details_add request;
		// a master admin approves it. Leaving this open to request.act meant an
		// admin could simply call the endpoint instead of filing the request, and
		// the approval chain would be a formality anyone could step around.
		//
		// It survives at all as the break-glass path for the case the request flow
		// cannot serve: ReqBankAdd cannot be executed from a payload, because the
		// account number never enters one (see request_execute_handler.go).
		admin.POST("/organizers/:id/payout-details",
			handlers.RequireMasterAdmin(pool),
			handlers.AdminSetOrganizerPayoutDetailsHandler(payoutService))
		admin.GET("/payouts",
			middleware.RequireCapabilityOn(pool, models.CapViewBankTail),
			handlers.AdminListPayoutsHandler(pool))
		admin.GET("/payouts/banks",
			middleware.RequireCapabilityOn(pool, models.CapViewBankTail),
			handlers.GetPayoutBanksHandler(payoutService))
		// Retrying moves money. Admin only.
		admin.POST("/payouts/:id/retry",
			middleware.RequireCapabilityOn(pool, models.CapActDangerous),
			handlers.AdminRetryPayoutHandler(pool, payoutService))
	}

	// ==========================================
	// Rate Limit Management (Admin)
	// ==========================================
	if rateLimitManager != nil {
		adminAPI := v1.Group("/admin/ratelimit")
		adminAPI.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
		{
			adminAPI.GET("/ip/:ip",             handlers.RateLimitIPStatusHandler(rateLimitManager))
			adminAPI.POST("/ip/:ip/reset",      handlers.RateLimitIPResetHandler(rateLimitManager))
			adminAPI.GET("/user/:userId",        handlers.RateLimitUserStatusHandler(rateLimitManager))
			adminAPI.POST("/user/:userId/reset", handlers.RateLimitUserResetHandler(rateLimitManager))
			adminAPI.GET("/metrics",             handlers.RateLimitMetricsHandler())
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
			"organiser"))
	}
	{
		orgRoutes.POST("/events/:id/publish", handlers.PublishEventHandler(pool, eventCache))
		orgRoutes.POST("/events/:id/cancel",  handlers.CancelEventHandler(pool, eventCache, refundService))
		orgRoutes.GET("/events",              handlers.GetOrganizerEventsHandler(pool))
		orgRoutes.POST("/events/create", handlers.OrgCreateEventHandler(pool, eventCache))

		// ── Editing an existing event ──────────────────────────────────────
		//
		// Split by lifecycle, not by convenience:
		//
		//   DELETE /events/:id           draft only, hard delete
		//   PUT    /events/:id           draft only, saves straight through
		//   GET    /events/:id/edit      full row + tiers, hydrates the form
		//   POST   /events/:id/edit-request
		//                                published only, files an event_edit
		//                                staff_request for admin approval
		//
		// A draft has no buyers and is not discoverable, so the organiser owns it
		// outright. A published event has people who paid against its current
		// terms, so changes go through review while it stays live and on sale.
		orgRoutes.DELETE("/events/:id",            handlers.DeleteDraftEventHandler(pool, eventCache))
		orgRoutes.PUT("/events/:id",               handlers.UpdateDraftEventHandler(pool, eventCache))
		orgRoutes.GET("/events/:id/edit",          handlers.GetOrganiserEventForEditHandler(pool))
		orgRoutes.POST("/events/:id/edit-request", handlers.SubmitOrganiserEventEditHandler(pool))
		orgRoutes.POST("/upload",         middleware.ExtendWriteTimeout(5*time.Minute), handlers.UploadOrganiserImageHandler(imgService))
		// Listing fee — required for events with 20+ ticket capacity
		orgRoutes.GET("/events/:id/listing-fee",           handlers.GetListingFeeHandler(pool))
		orgRoutes.POST("/events/:id/listing-fee/initiate", handlers.InitiateListingFeePaymentHandler(pool, paymentService))

		// Merch linked to an event
		orgRoutes.POST("/events/:id/merch",   handlers.OrgCreateMerchHandler(pool, eventCache))
		orgRoutes.PUT("/merch/:id",           handlers.OrgUpdateMerchHandler(pool, eventCache))
		orgRoutes.DELETE("/merch/:id",        handlers.OrgDeactivateMerchHandler(pool, eventCache))
		orgRoutes.POST("/merch/:id/variants", handlers.OrgCreateVariantHandler(pool, eventCache))
		orgRoutes.PUT("/merch/variants/:id",  handlers.OrgUpdateVariantHandler(pool, eventCache))
		// Standalone merch — not linked to any event
		orgRoutes.POST("/merch", handlers.OrgCreateStandaloneMerchHandler(pool, eventCache))
		// Assign / un-assign a merch item to a collection
		orgRoutes.PUT("/merch/:id/collection", handlers.OrgAssignMerchToCollectionHandler(pool))

		// Collection CRUD — organiser-scoped, ownership enforced
		orgRoutes.POST("/collections",           handlers.OrgCreateCollectionHandler(pool))
		orgRoutes.GET("/collections",            handlers.OrgGetCollectionsHandler(pool))
		orgRoutes.PUT("/collections/:id",        handlers.OrgUpdateCollectionHandler(pool))
		orgRoutes.DELETE("/collections/:id",     handlers.OrgDeactivateCollectionHandler(pool))
		orgRoutes.POST("/collections/:id/image", handlers.UploadCollectionCoverImageHandler(pool, imgService, false))

		// Ticket Types
		orgRoutes.GET("/events/:id/ticket-types",  handlers.OrgGetTicketTypesHandler(pool))
		orgRoutes.POST("/events/:id/ticket-types", handlers.OrgCreateTicketTypesHandler(pool))
		orgRoutes.PUT("/events/:id/ticket-types",  handlers.OrgReplaceTicketTypesHandler(pool))
		orgRoutes.PATCH("/ticket-types/:typeId",   handlers.OrgUpdateTicketTypeHandler(pool))
		orgRoutes.DELETE("/ticket-types/:typeId",  handlers.OrgDeleteTicketTypeHandler(pool))
		// ── Payouts ───────────────────────────────────────────────────────
		orgRoutes.GET("/payouts/banks",    handlers.GetPayoutBanksHandler(payoutService))
		orgRoutes.POST("/payouts/details", handlers.SetPayoutDetailsHandler(pool, payoutService))
		orgRoutes.GET("/payouts/profile",  handlers.GetPayoutProfileHandler(pool))
		orgRoutes.GET("/payouts/history",  handlers.GetPayoutHistoryHandler(pool))
		
		// ── Ads/campaigns/wallet ── DISABLED for launch (see note at ad worker) ─

		// ── Dashboard ─────────────────────────────────────────────────────────
		orgRoutes.GET("/dashboard/stats",               handlers.GetOrganizerDashboardStatsHandler(pool))
		orgRoutes.GET("/dashboard/events",              handlers.GetOrganizerDashboardEventsHandler(pool))
		orgRoutes.GET("/dashboard/events/:id/stats",    handlers.GetOrganizerEventStatsHandler(pool))
		orgRoutes.GET("/dashboard/events/:id/bookings", handlers.GetOrganizerEventBookingsHandler(pool))
		orgRoutes.GET("/dashboard/events/:id/merch",    handlers.GetOrganizerEventMerchHandler(pool))
		orgRoutes.GET("/dashboard/payouts",             handlers.GetOrganizerPayoutSummaryHandler(pool))

		// ── Analytics ─────────────────────────────────────────────────────
		// Reach, retention, PSR and door numbers. /overview backs the two
		// summary rows on the dashboard; the rest back the advanced page.
		//
		// Ownership is enforced inside every query in analytics_repository.go
		// rather than by a check in the handler, so a valid event id belonging
		// to another organiser returns nothing instead of leaking a row.
		orgRoutes.GET("/dashboard/analytics/overview",   handlers.GetAnalyticsOverviewHandler(pool))
		orgRoutes.GET("/dashboard/analytics/events",     handlers.ListEventAnalyticsHandler(pool))
		orgRoutes.GET("/dashboard/analytics/events/:id", handlers.GetEventAnalyticsHandler(pool))
		orgRoutes.GET("/dashboard/analytics/series",     handlers.GetAnalyticsSeriesHandler(pool))

		// Lists all the organiser's merch across their events. The dashboard
		// Merch page depended on this route but it was never registered --
		// both paths it tried 404'd, so the page rendered permanently empty.
		orgRoutes.GET("/merch", handlers.GetOrganizerMerchListHandler(pool))

		// Resubmit rejected merch for review
		orgRoutes.POST("/merch/:id/resubmit", handlers.OrgResubmitMerchHandler(pool))

		// ── Public organiser page (the organiser's own) ────────────────────
		//
		// GET   /organiser/public-profile                — including when not live
		// PATCH /organiser/public-profile                — description, cover, on/off
		// PUT   /organiser/public-profile/links          — replace the whole link set
		// GET   /organiser/public-profile/link-platforms — picker options
		//
		// NOT /organiser/profile. That path is already taken further down by
		// GetOrganizerProfileHandler, which returns the ACCOUNT record — legal
		// name, business email, payout state. Registering a second handler on
		// it would panic gin at startup. The two are genuinely different
		// things, so they get different paths: the account versus the public
		// page.
		//
		// These sit OUTSIDE the kyc_level >= 2 gate that governs whether the
		// page RESOLVES publicly. An organiser at level 1 can compose their
		// page and see exactly what it will look like; it simply does not go
		// live until they verify. Gating the editor too would leave them
		// staring at a dead screen with no way to prepare.
		orgRoutes.GET("/public-profile",                 handlers.GetMyPublicProfileHandler(pool))
		orgRoutes.PATCH("/public-profile",               handlers.UpdateMyPublicProfileHandler(pool))
		orgRoutes.PUT("/public-profile/links",           handlers.ReplaceMyLinksHandler(pool))
		orgRoutes.GET("/public-profile/link-platforms",  handlers.GetLinkPlatformsHandler())
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
	// Organiser Profile Media
	//
	// Separate from the event media group above, which is keyed on an :id the
	// organiser must own and whose handlers all verify that ownership through
	// GetEventImageURLs. A profile image has no event, so reusing that group
	// would mean inventing a fake event id or skipping the check that makes it
	// safe.
	//
	// Same tight bucket as event media — 20 burst, one refill per minute.
	// Image uploads are the most expensive thing an organiser can ask of us.
	// ==========================================
	profileMedia := v1.Group("/organiser/public-profile/media")
	profileMedia.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	profileMedia.Use(middleware.ExtendWriteTimeout(5 * time.Minute))
	if rateLimitManager != nil {
		profileMedia.Use(middleware.RateLimitByUser(rateLimitManager,
			ratelimit.BucketConfig{Capacity: 20, RefillRate: 1.0 / 60.0, TTL: 15 * time.Minute},
			"media-upload"))
	}
	{
		profileMedia.POST("/cover", handlers.UploadOrganizerCoverHandler(pool, imgService))
		profileMedia.POST("/logo",  handlers.UploadOrganizerLogoHandler(pool, imgService))
	}

	// ==========================================
	// Protected Test Route
	// ==========================================
	v1.GET("/protected-test", middleware.AuthMiddleware(cfg), handlers.TestProtectedHandler())

	// Organiser application — creates a PENDING profile; the organiser role
	// is only granted once KYC identity verification passes.
	v1.POST("/register-organizer",
		middleware.AuthMiddleware(cfg),
		handlers.RegisterAsOrganizerHandler(pool),
	)
	// KYC — auth-only (NOT organiser-only): applicants must be able to
	// submit identity verification BEFORE they hold the organiser role,
	// since passing KYC is now what grants the role.
	v1.GET("/organiser/kyc",
		middleware.AuthMiddleware(cfg),
		handlers.GetKYCStatusHandler(pool),
	)
	v1.POST("/organiser/kyc",
		middleware.AuthMiddleware(cfg),
		handlers.SubmitKYCHandler(pool, kycService),
	)
	v1.POST("/organiser/kyc/basic",
		middleware.AuthMiddleware(cfg),
		handlers.SubmitBasicKYCHandler(pool, kycService),
	)
	v1.GET("/organiser/profile",
		middleware.AuthMiddleware(cfg),
		middleware.OrganizerOnlyMiddleware(),
		handlers.GetOrganizerProfileHandler(pool),
	)

	// Legal acceptance — auth-only, NOT organiser-only, for the same reason as
	// KYC above: a pending applicant reaches /onboarding and /kyc before the
	// organiser role is granted, and has to be able to accept the agreements on
	// the way in. Gating these behind OrganizerOnly would mean the one group of
	// people being asked to agree couldn't record that they had.
	//
	// The dashboard gate FAILS OPEN when these are absent, so the frontend can
	// ship either side of this without locking anyone out of their dashboard.
	v1.GET("/organiser/legal",
		middleware.AuthMiddleware(cfg),
		handlers.GetLegalAcceptanceHandler(pool),
	)
	v1.POST("/organiser/legal/accept",
		middleware.AuthMiddleware(cfg),
		handlers.AcceptLegalHandler(pool),
	)

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