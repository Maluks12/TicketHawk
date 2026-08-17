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

	// ── Email (Amazon SES) ────────────────────────────────────────────────────
	// EmailFromAddress must be a verified SES identity (or belong to a verified
	// domain). EmailFromName is the display name. AWSRegion is the SES region
	// (af-south-1 for TicketHawk). SESAccessKeyID/SESSecretAccessKey are
	// optional — when empty, the AWS default credential chain is used (the EC2
	// instance role in production). SESConfigurationSet is optional and enables
	// SES event tracking (bounces/complaints/deliveries).
	EmailFromAddress    string
	EmailFromName       string
	AWSRegion           string
	SESAccessKeyID      string
	SESSecretAccessKey  string
	SESConfigurationSet string

	QREncryptionKey string // AES-GCM key for static QR codes (email/offline)
	KYCEncryptionKey string // AES-GCM key for stored SA ID numbers (falls back to QR key)
	DiditAPIKey      string // enables automatic DHA registry checks when set
	VerifyNowAPIKey  string // SA-local DHA + bank verification (verifynow.co.za)
	VerifyNowMode    string // "sandbox" (default) | "production"
	QRHMACSecret    string // HMAC-SHA256 secret for rotating in-app QR tokens

	R2AccountID string
	R2AccessKey string
	R2SecretKey string
	R2Bucket    string
	R2URL       string // S3-compatible API endpoint: https://<account-id>.r2.cloudflarestorage.com
	R2PublicURL string // Public serving URL: https://pub-xxx.r2.dev or https://cdn.yourdomain.com

	FrontendURLs []string

	// ── Social sign-in ───────────────────────────────────────────────────────
	// Comma-separated ACCEPTED AUDIENCES, not secrets. Each client platform has
	// its own OAuth client ID, and the ID token's `aud` claim carries whichever
	// one minted it — so the server has to accept all of them. Getting this
	// list wrong is the difference between "any app's Google token works" and
	// a correctly scoped check, which is why it is validated at boot rather
	// than defaulted.
	//
	//   GOOGLE_CLIENT_IDS=<web>.apps.googleusercontent.com,<ios>...,<android>...
	//   APPLE_CLIENT_IDS=com.tickethawk.app,com.tickethawk.web
	//
	// Apple is the subtle one: on iOS the audience is the BUNDLE ID, on web it
	// is the Services ID, and they are different strings.
	GoogleClientIDs []string
	AppleClientIDs  []string

	// OrganizerDashboardURL is where handover/invite emails point organisers
	// to sign in and set their password. Falls back to FrontendURLs[0] until
	// a dedicated dashboard domain exists.
	OrganizerDashboardURL string

	EnableEmailVerification bool
	EnableWaitlist          bool

	// ── Paystack (cards + Apple Pay) ──────────────────────────────────────────
	// SecretKey authenticates every server-to-server call (Bearer) and is also
	// the HMAC-SHA512 key Paystack uses to sign webhooks.
	PaystackSecretKey   string
	PaystackPublicKey   string
	PaystackEnvironment string // "test" | "live" (used only for logging/sanity)

	// WalletPaymentsEnabled gates the device-wallet methods (Apple Pay, Google
	// Pay, Samsung Pay). Default OFF for launch. These surface through Paystack
	// as normal card transactions, so the "gateway" isn't the issue — the flag
	// simply controls whether a client may select a wallet method. Apple Pay
	// for ZAR was impractical at last review; flip WALLET_PAYMENTS_ENABLED=true
	// once a wallet method is actually supported and tested.
	WalletPaymentsEnabled bool

	// ── PayPal (Orders v2) ────────────────────────────────────────────────────
	// PayPalEnabled gates whether PayPal can be selected as a payment method.
	// Default OFF: for a ZAR-only, SA-only launch PayPal adds FX risk, an FNB
	// dependency, a mandatory 30-day withdrawal window, and 4–6 day settlement,
	// with no split support — so it stays dormant until there's a clear reason
	// (e.g. meaningful international buyer volume) to turn it on. The gateway
	// code is fully retained; flip PAYPAL_ENABLED=true to re-enable, and
	// re-test against PayPal's sandbox first — dormant integrations can drift
	// out of date with the provider's API.
	PayPalEnabled      bool
	PayPalClientID     string
	PayPalClientSecret string
	PayPalWebhookID    string // configured in the PayPal dashboard; needed to verify webhooks
	PayPalEnvironment  string // "sandbox" | "live" — selects the api-m base URL

	// PaymentCallbackURL is where the gateway redirects the customer back to
	// after a hosted payment completes (e.g. https://api.yourdomain.com/payments/return
	// or a deep link bridge). Both gateways append their reference/token.
	PaymentCallbackURL string

	// ── Push notifications (Expo) ─────────────────────────────────────────────
	// PushEnabled gates the whole out-of-app delivery pipeline. Default ON.
	// Turning it OFF leaves the in-app notification feed completely untouched —
	// rows are still written, the app still lists them, unread counts still
	// work; only the phone buzz stops. That is the intended kill switch if
	// Expo is misbehaving or a notification storm needs stopping mid-flight.
	//
	// ExpoAccessToken is only REQUIRED when "enhanced security for push
	// notifications" is enabled on the Expo project (expo.dev → project →
	// Settings → Access tokens). Without that setting, Expo accepts unsigned
	// send requests and this can stay empty. Set it anyway if you have it:
	// otherwise anyone who obtains one of your push tokens can send
	// notifications that appear to come from TicketHawk.
	PushEnabled     bool
	ExpoAccessToken string

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

		EmailFromAddress:    os.Getenv("EMAIL_FROM_ADDRESS"),
		EmailFromName:       os.Getenv("EMAIL_FROM_NAME"),
		AWSRegion:           firstNonEmpty(os.Getenv("AWS_REGION"), "af-south-1"),
		SESAccessKeyID:      os.Getenv("AWS_SES_ACCESS_KEY_ID"),
		SESSecretAccessKey:  os.Getenv("AWS_SES_SECRET_ACCESS_KEY"),
		SESConfigurationSet: os.Getenv("SES_CONFIGURATION_SET"),

		QREncryptionKey: os.Getenv("QR_ENCRYPTION_KEY"),
		KYCEncryptionKey: firstNonEmpty(os.Getenv("KYC_ENCRYPTION_KEY"), os.Getenv("QR_ENCRYPTION_KEY")),
		DiditAPIKey:      os.Getenv("DIDIT_API_KEY"),
		VerifyNowAPIKey:  os.Getenv("VERIFYNOW_API_KEY"),
		VerifyNowMode:    os.Getenv("VERIFYNOW_MODE"),
		QRHMACSecret:    os.Getenv("QR_HMAC_SECRET"),

		R2AccountID: os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKey: os.Getenv("R2_ACCESS_KEY"),
		R2SecretKey: os.Getenv("R2_SECRET_KEY"),
		R2Bucket:    os.Getenv("R2_BUCKET"),
		R2URL:       os.Getenv("R2_URL"),
		R2PublicURL: os.Getenv("R2_PUBLIC_URL"),

		FrontendURLs: strings.Split(os.Getenv("FRONTEND_URLS"), ","),

		GoogleClientIDs: splitAndTrim(os.Getenv("GOOGLE_CLIENT_IDS")),
		AppleClientIDs:  splitAndTrim(os.Getenv("APPLE_CLIENT_IDS")),

		EnableEmailVerification: os.Getenv("ENABLE_EMAIL_VERIFICATION") == "true",
		EnableWaitlist:          os.Getenv("ENABLE_WAITLIST") == "true",

		PaystackSecretKey:   os.Getenv("PAYSTACK_SECRET_KEY"),
		PaystackPublicKey:   os.Getenv("PAYSTACK_PUBLIC_KEY"),
		PaystackEnvironment: os.Getenv("PAYSTACK_ENVIRONMENT"),

		WalletPaymentsEnabled: os.Getenv("WALLET_PAYMENTS_ENABLED") == "true",
		PayPalEnabled:      os.Getenv("PAYPAL_ENABLED") == "true",
		PayPalClientID:     os.Getenv("PAYPAL_CLIENT_ID"),
		PayPalClientSecret: os.Getenv("PAYPAL_CLIENT_SECRET"),
		PayPalWebhookID:    os.Getenv("PAYPAL_WEBHOOK_ID"),
		PayPalEnvironment:  os.Getenv("PAYPAL_ENVIRONMENT"),

		PaymentCallbackURL: os.Getenv("PAYMENT_CALLBACK_URL"),

		OrganizerDashboardURL: os.Getenv("ORGANIZER_DASHBOARD_URL"),

		// Default ON: unset means enabled. Note this is the opposite convention
		// to the other feature flags in this struct (`== "true"`), and
		// deliberately so — push should work on a fresh deploy without anyone
		// remembering to add an env var, and the flag exists to switch it OFF
		// in an emergency, not to switch it on.
		PushEnabled:     os.Getenv("PUSH_ENABLED") != "false",
		ExpoAccessToken: os.Getenv("EXPO_ACCESS_TOKEN"),
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
	if cfg.OrganizerDashboardURL == "" {
		// No dedicated dashboard domain yet — reuse the primary frontend URL
		// (validated non-empty above) so handover links always resolve.
		cfg.OrganizerDashboardURL = cfg.FrontendURLs[0]
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
	// SES sends from a verified identity; the From address is what must be set
	// (credentials come from the instance role or optional AWS_SES_* keys, and
	// the region defaults to af-south-1).
	if c.EmailFromAddress == "" {
		missing = append(missing, "EMAIL_FROM_ADDRESS")
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
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// splitAndTrim parses a comma-separated env var, dropping blanks. Returns an
// empty slice (not a one-element slice containing "") for an unset variable,
// which is what lets Enabled() distinguish "not configured" from "misconfigured".
func splitAndTrim(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}