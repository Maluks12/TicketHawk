package config

import (
	"errors"
	"log"
	"os"

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

	QREncryptionKey string

	R2AccountID string
	R2AccessKey string
	R2SecretKey string
	R2Bucket    string
	R2URL       string

	FrontendURL string

	EnableEmailVerification bool
	EnableWaitlist          bool

	StitchClientID      string
	StitchClientSecret  string
	StitchWebhookSecret string
	StitchEnvironment   string
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

		R2AccountID: os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKey: os.Getenv("R2_ACCESS_KEY"),
		R2SecretKey: os.Getenv("R2_SECRET_KEY"),
		R2Bucket:    os.Getenv("R2_BUCKET"),
		R2URL:       os.Getenv("R2_URL"),

		FrontendURL: os.Getenv("FRONTEND_URL"),

		EnableEmailVerification: os.Getenv("ENABLE_EMAIL_VERIFICATION") == "true",
		EnableWaitlist:          os.Getenv("ENABLE_WAITLIST") == "true",

		StitchClientID:      os.Getenv("STITCH_CLIENT_ID"),
		StitchClientSecret:  os.Getenv("STITCH_CLIENT_SECRET"),
		StitchWebhookSecret: os.Getenv("STITCH_WEBHOOK_SECRET"),
		StitchEnvironment:   os.Getenv("STITCH_ENVIRONMENT"),
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
	if cfg.StitchEnvironment == "" {
		cfg.StitchEnvironment = "sandbox"
	}

	return cfg, nil
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
	if c.StitchClientID == "" {
		missing = append(missing, "STITCH_CLIENT_ID")
	}
	if c.StitchClientSecret == "" {
		missing = append(missing, "STITCH_CLIENT_SECRET")
	}
	if c.StitchWebhookSecret == "" {
		missing = append(missing, "STITCH_WEBHOOK_SECRET")
	}
	if c.FrontendURL == "" {
		missing = append(missing, "FRONTEND_URL")
	}
	if c.SendGridAPIKey == "" {
		missing = append(missing, "SENDGRID_API_KEY")
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