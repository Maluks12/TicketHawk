package config

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	Port        string
	JWTSecret   string
	RedisAddr   string

	SendGridAPIKey   string
	EmailFromAddress string
	EmailFromName    string

	QREncryptionKey string // AES-GCM key for static QR codes (email/offline)
	QRHMACSecret    string // HMAC-SHA256 secret for rotating in-app QR tokens

	R2AccountID string
	R2AccessKey string
	R2SecretKey string
	R2Bucket    string
	R2URL       string // S3-compatible API endpoint: https://<account-id>.r2.cloudflarestorage.com
	R2PublicURL string // Public serving URL: https://pub-xxx.r2.dev or https://cdn.yourdomain.com

	FrontendURLs []string

	EnableEmailVerification bool
	EnableWaitlist          bool

	// ── Paystack (cards + Apple Pay) ──────────────────────────────────────────
	// SecretKey authenticates every server-to-server call (Bearer) and is also
	// the HMAC-SHA512 key Paystack uses to sign webhooks.
	PaystackSecretKey   string
	PaystackPublicKey   string
	PaystackEnvironment string // "test" | "live" (used only for logging/sanity)

	// ── PayPal (Orders v2) ────────────────────────────────────────────────────
	PayPalClientID     string
	PayPalClientSecret string
	PayPalWebhookID    string // configured in the PayPal dashboard; needed to verify webhooks
	PayPalEnvironment  string // "sandbox" | "live" — selects the api-m base URL

	// PaymentCallbackURL is where the gateway redirects the customer back to
	// after a hosted payment completes (e.g. https://api.yourdomain.com/payments/return
	// or a deep link bridge). Both gateways append their reference/token.
	PaymentCallbackURL string

	// Fix 3: booking expiry window — defaults to 15 minutes
	BookingExpiryMinutes int
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		RedisAddr:   os.Getenv("REDIS_ADDR"),

		SendGridAPIKey:   os.Getenv("SENDGRID_API_KEY"),
		EmailFromAddress: os.Getenv("EMAIL_FROM_ADDRESS"),
		EmailFromName:    os.Getenv("EMAIL_FROM_NAME"),

		QREncryptionKey: os.Getenv("QR_ENCRYPTION_KEY"),
		QRHMACSecret:    os.Getenv("QR_HMAC_SECRET"),

		R2AccountID: os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKey: os.Getenv("R2_ACCESS_KEY"),
		R2SecretKey: os.Getenv("R2_SECRET_KEY"),
		R2Bucket:    os.Getenv("R2_BUCKET"),
		R2URL:       os.Getenv("R2_URL"),
		R2PublicURL: os.Getenv("R2_PUBLIC_URL"),

		FrontendURLs: strings.Split(os.Getenv("FRONTEND_URLS"), ","),

		EnableEmailVerification: os.Getenv("ENABLE_EMAIL_VERIFICATION") == "true",
		EnableWaitlist:          os.Getenv("ENABLE_WAITLIST") == "true",

		PaystackSecretKey:   os.Getenv("PAYSTACK_SECRET_KEY"),
		PaystackPublicKey:   os.Getenv("PAYSTACK_PUBLIC_KEY"),
		PaystackEnvironment: os.Getenv("PAYSTACK_ENVIRONMENT"),

		PayPalClientID:     os.Getenv("PAYPAL_CLIENT_ID"),
		PayPalClientSecret: os.Getenv("PAYPAL_CLIENT_SECRET"),
		PayPalWebhookID:    os.Getenv("PAYPAL_WEBHOOK_ID"),
		PayPalEnvironment:  os.Getenv("PAYPAL_ENVIRONMENT"),

		PaymentCallbackURL: os.Getenv("PAYMENT_CALLBACK_URL"),
	}

	// FIX: validate required fields at startup so the server never silently
	// runs in a broken state (e.g. empty JWT_SECRET means all tokens are valid)
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Apply defaults for optional fields
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.PaystackEnvironment == "" {
		cfg.PaystackEnvironment = "test"
	}
	if cfg.PayPalEnvironment == "" {
		cfg.PayPalEnvironment = "sandbox"
	}

	cfg.BookingExpiryMinutes = 15
	if raw := os.Getenv("BOOKING_EXPIRY_MINUTES"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			cfg.BookingExpiryMinutes = parsed
		}
	}

	return cfg, nil
}

// PayPalBaseURL returns the api-m host for the configured environment.
func (c *Config) PayPalBaseURL() string {
	if strings.EqualFold(c.PayPalEnvironment, "live") {
		return "https://api-m.paypal.com"
	}
	return "https://api-m.sandbox.paypal.com"
}

func (c *Config) validate() error {
	var missing []string

	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters")
	}

	// Paystack — cards + Apple Pay for tickets, merch, listing fees, and wallet top-ups.
	if c.PaystackSecretKey == "" {
		missing = append(missing, "PAYSTACK_SECRET_KEY")
	}
	if c.PaystackPublicKey == "" {
		missing = append(missing, "PAYSTACK_PUBLIC_KEY")
	}

	// PayPal — consumer ticket/merch payments only (not wallet top-ups).
	if c.PayPalClientID == "" {
		missing = append(missing, "PAYPAL_CLIENT_ID")
	}
	if c.PayPalClientSecret == "" {
		missing = append(missing, "PAYPAL_CLIENT_SECRET")
	}
	if c.PayPalWebhookID == "" {
		missing = append(missing, "PAYPAL_WEBHOOK_ID")
	}

	if c.PaymentCallbackURL == "" {
		missing = append(missing, "PAYMENT_CALLBACK_URL")
	}
	if len(c.FrontendURLs) == 0 || c.FrontendURLs[0] == "" {
		missing = append(missing, "FRONTEND_URLS")
	}
	if c.SendGridAPIKey == "" {
		missing = append(missing, "SENDGRID_API_KEY")
	}
	if len(c.QRHMACSecret) < 32 {
		missing = append(missing, "QR_HMAC_SECRET (must be at least 32 chars — generate with: openssl rand -hex 32)")
	}

	if len(missing) > 0 {
		msg := "missing required environment variables:"
		for _, v := range missing {
			msg += "\n  - " + v
		}
		return errors.New(msg)
	}

	return nil
}