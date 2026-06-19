package handlers_test

// =============================================================================
// AUTH FLOW TESTS
// =============================================================================
//
// Tests covered:
//   - POST /auth/register   → CreateUserhandler
//   - POST /auth/login      → LoginHandler
//   - POST /auth/refresh    → RefreshTokenHandler
//   - POST /auth/logout     → LogoutHandler
//   - POST /auth/reset_password_request → RequestPasswordResetHandler
//   - POST /auth/reset_password         → ResetPasswordHandler
//
// Prerequisites (run once before the suite):
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
//   Apply all migrations to test_db first, OR let the helper below create
//   the minimal schema it needs.
//
// Run tests:
//
//   go test ./internal/handlers/... -v -run TestAuth
// =============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestMain registers the app's custom binding validators (strong_password, etc.)
// with gin's shared validator engine before any test runs — production does this
// once in main(), and the DTOs the auth handlers bind depend on it.
func TestMain(m *testing.M) {
	utils.RegisterCustomValidators()
	os.Exit(m.Run())
}

// ─── shared test setup ───────────────────────────────────────────────────────

const authTestDBURL = "postgresql://testuser:testpass@localhost:5433/test_db"

// authTestConfig builds a config for tests. The payment provider fields are
// Paystack (cards + Apple Pay) and PayPal — Stitch has been removed. The
// PaystackSecretKey doubles as the webhook HMAC-SHA512 key, so webhook tests
// can sign payloads against it.
func authTestConfig() *config.Config {
	return &config.Config{
		JWTSecret:        "test-secret-that-is-at-least-32-chars!!",
		FrontendURLs:     []string{"http://localhost:3000"},
		SendGridAPIKey:   "SG.test",
		EmailFromAddress: "noreply@test.com",
		EmailFromName:    "Test",

		PaystackSecretKey:   "sk_test_paystack_secret_key",
		PaystackPublicKey:   "pk_test_paystack_public_key",
		PaystackEnvironment: "test",

		PayPalClientID:     "paypal-client-id",
		PayPalClientSecret: "paypal-client-secret",
		PayPalWebhookID:    "paypal-webhook-id",
		PayPalEnvironment:  "sandbox",

		PaymentCallbackURL: "http://localhost:3000/payments/return",
	}
}

func setupAuthTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Fatalf(
			"Cannot connect to test DB: %v\n"+
				"Start it with:\n"+
				"  docker run -d --name test-postgres"+
				" -e POSTGRES_USER=testuser"+
				" -e POSTGRES_PASSWORD=testpass"+
				" -e POSTGRES_DB=test_db"+
				" -p 5433:5432 postgres:15",
			err,
		)
	}
	createAuthTestTables(t, pool)
	cleanAuthTables(t, pool)
	return pool
}

func createAuthTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Drop-and-recreate (not IF NOT EXISTS) so a stale, thinner users table from
	// an earlier run can't linger without the columns the repository writes.
	for _, s := range []string{
		"DROP TABLE IF EXISTS password_resets CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens  CASCADE",
		"DROP TABLE IF EXISTS users           CASCADE",
	} {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		// CreateUser inserts first_name, last_name, phone_number; the various
		// SELECTs read role and created_at — every column must exist here.
		`CREATE TABLE users (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email        VARCHAR(255) UNIQUE NOT NULL,
			password     TEXT NOT NULL,
			role         VARCHAR(50) NOT NULL DEFAULT 'user',
			first_name   VARCHAR(100),
			last_name    VARCHAR(100),
			phone_number VARCHAR(50),
			created_at   TIMESTAMPTZ DEFAULT NOW(),
			updated_at   TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE password_resets (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash  TEXT NOT NULL,
			expires_at  TIMESTAMPTZ NOT NULL,
			used        BOOLEAN DEFAULT FALSE,
			created_at  TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE refresh_tokens (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash  TEXT UNIQUE NOT NULL,
			expires_at  TIMESTAMPTZ NOT NULL,
			revoked     BOOLEAN DEFAULT FALSE,
			created_at  TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("auth table setup failed: %v\nSQL: %s", err, s)
		}
	}
}

func cleanAuthTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	pool.Exec(context.Background(), "TRUNCATE password_resets CASCADE")
	pool.Exec(context.Background(), "TRUNCATE refresh_tokens  CASCADE")
	pool.Exec(context.Background(), "TRUNCATE users           CASCADE")
}

func randomEmail() string {
	return fmt.Sprintf("user%d_%d@test.com", rand.Intn(1_000_000), time.Now().UnixNano())
}

// validRegisterPayload returns a registration body satisfying every required
// field on RegisterRequest (email, password, first_name, last_name, phone_number).
func validRegisterPayload(email string) map[string]string {
	return map[string]string{
		"email":        email,
		"password":     "SecurePass1!",
		"first_name":   "Test",
		"last_name":    "User",
		"phone_number": "+27123456789",
	}
}

// buildAuthRouter wires up all auth routes (no rate limiting).
func buildAuthRouter(pool *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	emailSvc := services.NewEmailService(cfg)
	auth := r.Group("/auth")
	auth.POST("/register", handlers.CreateUserhandler(pool))
	auth.POST("/login", handlers.LoginHandler(pool, cfg))
	auth.POST("/refresh", handlers.RefreshTokenHandler(pool, cfg))
	auth.POST("/logout", middleware.AuthMiddleware(cfg), handlers.LogoutHandler(pool))
	auth.POST("/reset_password_request", handlers.RequestPasswordResetHandler(pool, emailSvc, cfg))
	auth.POST("/reset_password", handlers.ResetPasswordHandler(pool))
	return r
}

// doRequest is a convenience helper for JSON requests.
func doRequest(r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// doAuthRequest attaches a Bearer token.
func doAuthRequest(r *gin.Engine, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// loginAndGetTokens registers + logs in a fresh user and returns tokens.
func loginAndGetTokens(t *testing.T, pool *pgxpool.Pool, cfg *config.Config) (accessToken, refreshToken string) {
	t.Helper()
	email, password := randomEmail(), "Password1234!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	u := &models.User{Email: email, Password: string(hashed), Role: "user"}
	_, err := repository.CreateUser(pool, u)
	require.NoError(t, err)

	r := buildAuthRouter(pool, cfg)
	w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": password,
	})
	require.Equal(t, http.StatusOK, w.Code, "login failed: %s", w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	accessToken = resp["access_token"].(string)
	refreshToken = resp["refresh_token"].(string)
	return
}

// ─── Registration tests ───────────────────────────────────────────────────────

func TestAuthRegister(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()
	r := buildAuthRouter(pool, cfg)

	t.Run("success — new user gets 201 with email in response", func(t *testing.T) {
		cleanAuthTables(t, pool)
		email := randomEmail()
		w := doRequest(r, http.MethodPost, "/auth/register", validRegisterPayload(email))
		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, email, resp["email"])
		assert.NotEmpty(t, resp["id"])
	})

	t.Run("duplicate email returns 400", func(t *testing.T) {
		cleanAuthTables(t, pool)
		email := randomEmail()
		w := doRequest(r, http.MethodPost, "/auth/register", validRegisterPayload(email))
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		w = doRequest(r, http.MethodPost, "/auth/register", validRegisterPayload(email))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["error"], "already registered")
	})

	t.Run("missing email returns 400 validation error", func(t *testing.T) {
		body := validRegisterPayload(randomEmail())
		delete(body, "email")
		w := doRequest(r, http.MethodPost, "/auth/register", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("password too short (< 8 chars) returns 400", func(t *testing.T) {
		body := validRegisterPayload(randomEmail())
		body["password"] = "short"
		w := doRequest(r, http.MethodPost, "/auth/register", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid email format returns 400", func(t *testing.T) {
		body := validRegisterPayload("not-an-email")
		w := doRequest(r, http.MethodPost, "/auth/register", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty body returns 400", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/register", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── Login tests ──────────────────────────────────────────────────────────────

func TestAuthLogin(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()
	r := buildAuthRouter(pool, cfg)

	cleanAuthTables(t, pool)
	const knownEmail = "login_test@test.com"
	const knownPassword = "LoginPass1234!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(knownPassword), bcrypt.DefaultCost)
	_, err := repository.CreateUser(pool, &models.User{
		Email: knownEmail, Password: string(hashed), Role: "user",
	})
	require.NoError(t, err)

	t.Run("success — returns access_token and refresh_token", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": knownEmail, "password": knownPassword,
		})
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["access_token"], "access_token must be present")
		assert.NotEmpty(t, resp["refresh_token"], "refresh_token must be present")
		assert.EqualValues(t, 900, resp["expires_in"], "access token TTL should be 900s")
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": knownEmail, "password": "WrongPassword!",
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Invalid credentials", resp["error"])
	})

	t.Run("non-existent email returns 401", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": "ghost@test.com", "password": knownPassword,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("missing email field returns 400", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"password": knownPassword,
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing password field returns 400", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": knownEmail,
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── Token refresh tests ──────────────────────────────────────────────────────

func TestAuthRefreshToken(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	t.Run("valid refresh_token returns a new access_token", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, refreshToken := loginAndGetTokens(t, pool, cfg)
		r := buildAuthRouter(pool, cfg)

		w := doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": refreshToken,
		})
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["access_token"])
		assert.NotEmpty(t, resp["refresh_token"], "a new refresh token must be issued")
	})

	t.Run("refresh token can only be used once (rotation)", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, refreshToken := loginAndGetTokens(t, pool, cfg)
		r := buildAuthRouter(pool, cfg)

		w := doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": refreshToken,
		})
		require.Equal(t, http.StatusOK, w.Code)

		w = doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": refreshToken,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code, "replayed refresh token must be rejected")
	})

	t.Run("invalid refresh_token returns 401", func(t *testing.T) {
		cleanAuthTables(t, pool)
		r := buildAuthRouter(pool, cfg)

		w := doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": "totally-made-up-garbage",
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("missing refresh_token returns 400", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── Logout tests ─────────────────────────────────────────────────────────────

func TestAuthLogout(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	t.Run("logout with valid token returns 200", func(t *testing.T) {
		cleanAuthTables(t, pool)
		accessToken, _ := loginAndGetTokens(t, pool, cfg)
		r := buildAuthRouter(pool, cfg)

		w := doAuthRequest(r, http.MethodPost, "/auth/logout", nil, accessToken)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Logged out successfully", resp["message"])
	})

	t.Run("logout without token returns 401", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("logout with malformed Bearer token returns 401", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doAuthRequest(r, http.MethodPost, "/auth/logout", nil, "not.a.real.jwt")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── Password reset tests ─────────────────────────────────────────────────────

func TestAuthPasswordReset(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	t.Run("reset request for existing email returns generic 200 (no enumeration)", func(t *testing.T) {
		cleanAuthTables(t, pool)
		email := randomEmail()
		hashed, _ := bcrypt.GenerateFromPassword([]byte("OldPass123!"), bcrypt.DefaultCost)
		_, err := repository.CreateUser(pool, &models.User{Email: email, Password: string(hashed), Role: "user"})
		require.NoError(t, err)

		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password_request", map[string]string{
			"email": email,
		})
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["message"])
	})

	t.Run("reset request for non-existent email also returns 200 (anti-enumeration)", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password_request", map[string]string{
			"email": "ghost_nobody@test.com",
		})
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("reset with invalid UUID token returns 400", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token":        "not-a-uuid",
			"new_password": "NewSecurePass1!",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("reset with expired/non-existent token returns 400", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token":        "550e8400-e29b-41d4-a716-446655440000",
			"new_password": "NewSecurePass1!",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("new password too short returns 400", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token":        "550e8400-e29b-41d4-a716-446655440000",
			"new_password": "short",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── JWT middleware tests ─────────────────────────────────────────────────────

func TestAuthMiddleware(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", middleware.AuthMiddleware(cfg), handlers.TestProtectedHandler())
	r.POST("/auth/login", handlers.LoginHandler(pool, cfg))

	t.Run("no Authorization header returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Authorization header without Bearer prefix returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Token some-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("valid JWT gives access to protected route", func(t *testing.T) {
		cleanAuthTables(t, pool)
		accessToken, _ := loginAndGetTokens(t, pool, cfg)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("tampered JWT signature returns 401", func(t *testing.T) {
		cleanAuthTables(t, pool)
		accessToken, _ := loginAndGetTokens(t, pool, cfg)
		tampered := accessToken[:len(accessToken)-4] + "xxxx"

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tampered)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}