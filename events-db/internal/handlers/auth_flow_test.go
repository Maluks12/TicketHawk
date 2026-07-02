package handlers_test

// =============================================================================
// AUTH FLOW TESTS — comprehensive suite
// =============================================================================
//
// Endpoints covered:
//   - POST /auth/register               → CreateUserhandler
//   - POST /auth/login                  → LoginHandler
//   - POST /auth/refresh                → RefreshTokenHandler
//   - POST /auth/logout                 → LogoutHandler
//   - POST /auth/reset_password_request → RequestPasswordResetHandler
//   - POST /auth/reset_password         → ResetPasswordHandler
//
// Middleware covered:
//   - AuthMiddleware / OptionalAuth / OrganizerOnly / AdminOnly
//   - RateLimitAuthByIP / RateLimitLoginByEmail (in-memory fallback path)
//
// Security properties asserted (mapped to the hardening fixes):
//   FIX 1 — password reset revokes every refresh token
//   FIX 2 — refresh token reuse revokes the whole session family
//   FIX 3 — unknown-email login burns a bcrypt compare (timing equalization)
//   FIX 4 — auth rate limiting works with NO Redis (nil manager fallback)
//   Plus: mass-assignment (role escalation), alg-confusion ("none" JWT),
//   cross-secret JWTs, expired JWTs, hashed-at-rest refresh tokens,
//   no password/hash leakage in responses, SQL-injection smoke tests,
//   single-use reset tokens, logout revoking all devices.
//
// Prerequisites (run once before the suite):
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
// Run:
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
	"strings"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/ratelimit"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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

	// Drop-and-recreate (not IF NOT EXISTS) so a stale, thinner table from an
	// earlier run can't linger without the columns the repository writes.
	for _, s := range []string{
		"DROP TABLE IF EXISTS password_resets CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens  CASCADE",
		"DROP TABLE IF EXISTS users           CASCADE",
	} {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		`CREATE TABLE users (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email        VARCHAR(255) UNIQUE NOT NULL,
			password     TEXT NOT NULL,
			role         VARCHAR(50) NOT NULL DEFAULT 'attendee',
			first_name   VARCHAR(100),
			last_name    VARCHAR(100),
			phone_number VARCHAR(50),
			created_at   TIMESTAMPTZ DEFAULT NOW(),
			updated_at   TIMESTAMPTZ DEFAULT NOW()
		)`,
		// NOTE: used_at (not the old boolean "used") — repository.ResetPassword
		// checks used_at IS NULL and stamps used_at = NOW() on consumption.
		`CREATE TABLE password_resets (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash  TEXT NOT NULL,
			expires_at  TIMESTAMPTZ NOT NULL,
			used_at     TIMESTAMPTZ,
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
// field on RegisterRequest.
func validRegisterPayload(email string) map[string]string {
	return map[string]string{
		"email":        email,
		"password":     "SecurePass1!",
		"first_name":   "Test",
		"last_name":    "User",
		"phone_number": "+27123456789",
	}
}

// buildAuthRouter wires up all auth routes (no rate limiting — the rate limit
// middleware has its own dedicated tests below).
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

// createTestUser inserts a user directly and returns (id, email).
func createTestUser(t *testing.T, pool *pgxpool.Pool, password string, role models.UserRole) (string, string) {
	t.Helper()
	email := randomEmail()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)
	u := &models.User{Email: email, Password: string(hashed), Role: role}
	created, err := repository.CreateUser(pool, u)
	require.NoError(t, err)
	return created.ID, email
}

// loginAndGetTokens registers + logs in a fresh user and returns tokens.
func loginAndGetTokens(t *testing.T, pool *pgxpool.Pool, cfg *config.Config) (accessToken, refreshToken string) {
	t.Helper()
	accessToken, refreshToken, _, _ = loginAndGetTokensWithUser(t, pool, cfg)
	return
}

// loginAndGetTokensWithUser is loginAndGetTokens but also returns identity.
func loginAndGetTokensWithUser(t *testing.T, pool *pgxpool.Pool, cfg *config.Config) (accessToken, refreshToken, userID, email string) {
	t.Helper()
	const password = "Password1234!"
	userID, email = createTestUser(t, pool, password, models.RoleAttendee)

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

// mintJWT builds an HS256 token with arbitrary claims and secret — used to
// probe the middleware with forged, expired, and malformed tokens.
func mintJWT(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

// ═══════════════════════════════════════════════════════════════════════════
// Registration
// ═══════════════════════════════════════════════════════════════════════════

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

	t.Run("response never leaks the password or its hash", func(t *testing.T) {
		cleanAuthTables(t, pool)
		payload := validRegisterPayload(randomEmail())
		w := doRequest(r, http.MethodPost, "/auth/register", payload)
		require.Equal(t, http.StatusCreated, w.Code)
		body := w.Body.String()
		assert.NotContains(t, body, payload["password"], "plaintext password leaked")
		assert.NotContains(t, body, "$2a$", "bcrypt hash leaked")
		assert.NotContains(t, body, "$2b$", "bcrypt hash leaked")
	})

	t.Run("password is stored bcrypt-hashed, never plaintext", func(t *testing.T) {
		cleanAuthTables(t, pool)
		payload := validRegisterPayload(randomEmail())
		w := doRequest(r, http.MethodPost, "/auth/register", payload)
		require.Equal(t, http.StatusCreated, w.Code)

		var stored string
		err := pool.QueryRow(context.Background(),
			"SELECT password FROM users WHERE email = $1", payload["email"]).Scan(&stored)
		require.NoError(t, err)
		assert.NotEqual(t, payload["password"], stored, "password stored in plaintext")
		assert.True(t, strings.HasPrefix(stored, "$2"), "stored password is not a bcrypt hash: %s", stored)
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored), []byte(payload["password"])))
	})

	t.Run("mass assignment — role in payload is ignored, user is always attendee", func(t *testing.T) {
		cleanAuthTables(t, pool)
		email := randomEmail()
		payload := map[string]string{
			"email":        email,
			"password":     "SecurePass1!",
			"first_name":   "Evil",
			"last_name":    "Admin",
			"phone_number": "+27123456789",
			"role":         "admin", // must be silently discarded
		}
		w := doRequest(r, http.MethodPost, "/auth/register", payload)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var role string
		err := pool.QueryRow(context.Background(),
			"SELECT role FROM users WHERE email = $1", email).Scan(&role)
		require.NoError(t, err)
		assert.Equal(t, string(models.RoleAttendee), role, "privilege escalation via register payload")
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

	t.Run("strong_password — rejects missing uppercase / digit / special / length", func(t *testing.T) {
		weak := []string{
			"alllowercase1!", // no uppercase
			"NoDigitsHere!",  // no digit
			"NoSpecial123A",  // no special char
			"Sh0r!t",         // < 8 chars
			"password",       // fails several rules
		}
		for _, pw := range weak {
			body := validRegisterPayload(randomEmail())
			body["password"] = pw
			w := doRequest(r, http.MethodPost, "/auth/register", body)
			assert.Equalf(t, http.StatusBadRequest, w.Code, "weak password accepted: %q", pw)
		}
	})

	t.Run("invalid email format returns 400", func(t *testing.T) {
		for _, bad := range []string{"not-an-email", "a@", "@b.com", "a b@c.com"} {
			body := validRegisterPayload(bad)
			w := doRequest(r, http.MethodPost, "/auth/register", body)
			assert.Equalf(t, http.StatusBadRequest, w.Code, "invalid email accepted: %q", bad)
		}
	})

	t.Run("oversized email (>255 chars) returns 400", func(t *testing.T) {
		body := validRegisterPayload(strings.Repeat("a", 250) + "@test.com")
		w := doRequest(r, http.MethodPost, "/auth/register", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields each return 400", func(t *testing.T) {
		for _, field := range []string{"email", "password", "first_name", "last_name", "phone_number"} {
			body := validRegisterPayload(randomEmail())
			delete(body, field)
			w := doRequest(r, http.MethodPost, "/auth/register", body)
			assert.Equalf(t, http.StatusBadRequest, w.Code, "missing %s was accepted", field)
		}
	})

	t.Run("empty body returns 400", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/register", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed JSON returns 400, not 500", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/register",
			strings.NewReader(`{"email": "broken@test.com", "password":`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// Login
// ═══════════════════════════════════════════════════════════════════════════

func TestAuthLogin(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()
	r := buildAuthRouter(pool, cfg)

	cleanAuthTables(t, pool)
	const knownPassword = "LoginPass1234!"
	_, knownEmail := createTestUser(t, pool, knownPassword, models.RoleAttendee)

	t.Run("success — returns access_token, refresh_token, expires_in", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": knownEmail, "password": knownPassword,
		})
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["access_token"])
		assert.NotEmpty(t, resp["refresh_token"])
		assert.EqualValues(t, 900, resp["expires_in"], "access token TTL should be 900s")
	})

	t.Run("access token claims — HS256, correct identity, ~15 min expiry", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": knownEmail, "password": knownPassword,
		})
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		tok, err := jwt.Parse(resp["access_token"].(string), func(tk *jwt.Token) (interface{}, error) {
			require.Equal(t, jwt.SigningMethodHS256.Alg(), tk.Method.Alg(), "unexpected signing algorithm")
			return []byte(cfg.JWTSecret), nil
		})
		require.NoError(t, err)
		require.True(t, tok.Valid)

		claims := tok.Claims.(jwt.MapClaims)
		assert.Equal(t, knownEmail, claims["email"])
		assert.Equal(t, string(models.RoleAttendee), claims["role"])
		assert.NotEmpty(t, claims["user_id"])

		exp := int64(claims["exp"].(float64))
		ttl := time.Until(time.Unix(exp, 0))
		assert.Greater(t, ttl, 13*time.Minute, "expiry too short")
		assert.LessOrEqual(t, ttl, 15*time.Minute+30*time.Second, "expiry too long")
	})

	t.Run("refresh token is stored hashed at rest (sha256), never raw", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, refreshToken, userID, _ := loginAndGetTokensWithUser(t, pool, cfg)

		var storedHash string
		err := pool.QueryRow(context.Background(),
			"SELECT token_hash FROM refresh_tokens WHERE user_id = $1", userID).Scan(&storedHash)
		require.NoError(t, err)
		assert.NotEqual(t, refreshToken, storedHash, "refresh token stored in plaintext")
		assert.Equal(t, utils.HashResetToken(refreshToken), storedHash,
			"stored hash must be sha256(raw token)")
	})

	t.Run("wrong password returns 401 with generic message", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": knownEmail, "password": "WrongPassword1!",
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Invalid credentials", resp["error"])
	})

	t.Run("non-existent email returns the SAME 401 body as wrong password (no enumeration)", func(t *testing.T) {
		wUnknown := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": "ghost@test.com", "password": knownPassword,
		})
		wWrongPw := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": knownEmail, "password": "WrongPassword1!",
		})
		assert.Equal(t, http.StatusUnauthorized, wUnknown.Code)
		assert.Equal(t, http.StatusUnauthorized, wWrongPw.Code)
		assert.JSONEq(t, wWrongPw.Body.String(), wUnknown.Body.String(),
			"401 bodies must be identical for unknown email vs wrong password")
	})

	t.Run("FIX 3 — unknown-email login burns a bcrypt compare (no timing oracle)", func(t *testing.T) {
		// A bcrypt compare at DefaultCost takes tens of milliseconds. Before
		// the fix, the unknown-email path returned in well under a
		// millisecond. Asserting a generous floor (15ms) proves the dummy
		// compare runs without making the test flaky on slow CI machines.
		start := time.Now()
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": "definitely-not-registered@test.com", "password": "SomePassword1!",
		})
		elapsed := time.Since(start)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.GreaterOrEqual(t, elapsed, 15*time.Millisecond,
			"unknown-email login returned too fast — dummy bcrypt compare is missing (timing side-channel)")
	})

	t.Run("SQL injection in email field is handled safely (400/401, never 500)", func(t *testing.T) {
		for _, inj := range []string{
			"' OR '1'='1",
			"admin'--@test.com",
			"x@test.com'; DROP TABLE users;--",
		} {
			w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
				"email": inj, "password": "Whatever123!",
			})
			assert.Containsf(t, []int{http.StatusBadRequest, http.StatusUnauthorized}, w.Code,
				"injection payload %q caused status %d", inj, w.Code)
		}
		// users table must still exist and be queryable
		var n int
		require.NoError(t, pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users").Scan(&n))
	})

	t.Run("missing email field returns 400", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{"password": knownPassword})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing password field returns 400", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{"email": knownEmail})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("each login issues a distinct refresh token (multi-device)", func(t *testing.T) {
		// Earlier subtests truncate the tables, so use a fresh user rather
		// than the shared fixture from the top of the suite.
		_, email := createTestUser(t, pool, knownPassword, models.RoleAttendee)
		body := map[string]string{"email": email, "password": knownPassword}
		w1 := doRequest(r, http.MethodPost, "/auth/login", body)
		w2 := doRequest(r, http.MethodPost, "/auth/login", body)
		require.Equal(t, http.StatusOK, w1.Code)
		require.Equal(t, http.StatusOK, w2.Code)
		var r1, r2 map[string]interface{}
		require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &r1))
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &r2))
		assert.NotEqual(t, r1["refresh_token"], r2["refresh_token"])
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// Refresh token rotation & reuse detection
// ═══════════════════════════════════════════════════════════════════════════

func TestAuthRefreshToken(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	refresh := func(r *gin.Engine, token string) *httptest.ResponseRecorder {
		return doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": token,
		})
	}

	t.Run("valid refresh_token returns a new access_token + rotated refresh_token", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, refreshToken := loginAndGetTokens(t, pool, cfg)
		r := buildAuthRouter(pool, cfg)

		w := refresh(r, refreshToken)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["access_token"])
		assert.NotEmpty(t, resp["refresh_token"])
		assert.NotEqual(t, refreshToken, resp["refresh_token"], "refresh token must rotate")
	})

	t.Run("refresh token can only be used once (rotation)", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, refreshToken := loginAndGetTokens(t, pool, cfg)
		r := buildAuthRouter(pool, cfg)

		require.Equal(t, http.StatusOK, refresh(r, refreshToken).Code)
		assert.Equal(t, http.StatusUnauthorized, refresh(r, refreshToken).Code,
			"replayed refresh token must be rejected")
	})

	t.Run("FIX 2 — replaying a rotated token revokes the WHOLE session family", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, tokenA := loginAndGetTokens(t, pool, cfg)
		r := buildAuthRouter(pool, cfg)

		// Legitimate rotation: A → B
		w := refresh(r, tokenA)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		tokenB := resp["refresh_token"].(string)

		// Attacker replays A → 401, and reuse detection must fire
		assert.Equal(t, http.StatusUnauthorized, refresh(r, tokenA).Code)

		// The currently-valid token B must ALSO be dead now
		assert.Equal(t, http.StatusUnauthorized, refresh(r, tokenB).Code,
			"reuse detection did not revoke the live token — stolen-token replay leaves attacker sessions alive")
	})

	t.Run("expired refresh token returns 401", func(t *testing.T) {
		cleanAuthTables(t, pool)
		userID, _ := createTestUser(t, pool, "Password1234!", models.RoleAttendee)

		raw, hashed, err := utils.GenerateResetToken()
		require.NoError(t, err)
		require.NoError(t, repository.CreateRefreshToken(pool, userID, hashed, time.Now().Add(-1*time.Hour)))

		r := buildAuthRouter(pool, cfg)
		assert.Equal(t, http.StatusUnauthorized, refresh(r, raw).Code)
	})

	t.Run("invalid refresh_token returns 401", func(t *testing.T) {
		cleanAuthTables(t, pool)
		r := buildAuthRouter(pool, cfg)
		assert.Equal(t, http.StatusUnauthorized, refresh(r, "totally-made-up-garbage").Code)
	})

	t.Run("missing refresh_token returns 400", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("refresh token of a deleted user returns 401", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, refreshToken, userID, _ := loginAndGetTokensWithUser(t, pool, cfg)
		require.NoError(t, repository.DeleteUserByID(pool, userID))

		r := buildAuthRouter(pool, cfg)
		assert.Equal(t, http.StatusUnauthorized, refresh(r, refreshToken).Code)
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// Logout
// ═══════════════════════════════════════════════════════════════════════════

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

	t.Run("logout revokes the refresh token — refresh afterwards returns 401", func(t *testing.T) {
		cleanAuthTables(t, pool)
		accessToken, refreshToken := loginAndGetTokens(t, pool, cfg)
		r := buildAuthRouter(pool, cfg)

		require.Equal(t, http.StatusOK,
			doAuthRequest(r, http.MethodPost, "/auth/logout", nil, accessToken).Code)

		w := doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": refreshToken,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code, "refresh token must be dead after logout")
	})

	t.Run("logout revokes ALL sessions (every device)", func(t *testing.T) {
		cleanAuthTables(t, pool)
		const password = "Password1234!"
		_, email := createTestUser(t, pool, password, models.RoleAttendee)
		r := buildAuthRouter(pool, cfg)
		body := map[string]string{"email": email, "password": password}

		// Two logins = two devices = two refresh tokens
		w1 := doRequest(r, http.MethodPost, "/auth/login", body)
		w2 := doRequest(r, http.MethodPost, "/auth/login", body)
		require.Equal(t, http.StatusOK, w1.Code)
		require.Equal(t, http.StatusOK, w2.Code)
		var s1, s2 map[string]interface{}
		require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &s1))
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &s2))

		// Logout from device 1
		require.Equal(t, http.StatusOK,
			doAuthRequest(r, http.MethodPost, "/auth/logout", nil, s1["access_token"].(string)).Code)

		// Both refresh tokens must now be revoked
		for i, s := range []map[string]interface{}{s1, s2} {
			w := doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
				"refresh_token": s["refresh_token"].(string),
			})
			assert.Equalf(t, http.StatusUnauthorized, w.Code, "device %d refresh token survived logout", i+1)
		}
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

// ═══════════════════════════════════════════════════════════════════════════
// Password reset — full flow, single use, session revocation
// ═══════════════════════════════════════════════════════════════════════════

func TestAuthPasswordReset(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	// createResetToken inserts a valid reset row and returns the raw token
	// (mirrors what RequestPasswordResetHandler emails to the user).
	createResetToken := func(t *testing.T, userID string, expiresAt time.Time) string {
		t.Helper()
		raw, hashed, err := utils.GenerateResetToken()
		require.NoError(t, err)
		require.NoError(t, repository.CreatePasswordReset(pool, userID, hashed, expiresAt))
		return raw
	}

	t.Run("reset request for existing email returns generic 200 (no enumeration)", func(t *testing.T) {
		cleanAuthTables(t, pool)
		_, email := createTestUser(t, pool, "OldPass123!", models.RoleAttendee)

		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password_request", map[string]string{"email": email})
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

	t.Run("full flow — reset succeeds, new password works, old password rejected", func(t *testing.T) {
		cleanAuthTables(t, pool)
		const oldPassword = "OldPassword1!"
		const newPassword = "NewPassword2@"
		userID, email := createTestUser(t, pool, oldPassword, models.RoleAttendee)
		rawToken := createResetToken(t, userID, time.Now().Add(30*time.Minute))

		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token": rawToken, "new_password": newPassword,
		})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// old password no longer works
		w = doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": email, "password": oldPassword,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code, "old password still valid after reset")

		// new password works
		w = doRequest(r, http.MethodPost, "/auth/login", map[string]string{
			"email": email, "password": newPassword,
		})
		assert.Equal(t, http.StatusOK, w.Code, "new password rejected after reset: %s", w.Body.String())
	})

	t.Run("FIX 1 — password reset revokes every existing refresh token", func(t *testing.T) {
		cleanAuthTables(t, pool)
		// User logs in (attacker has stolen this session's refresh token)
		_, stolenRefresh, userID, _ := loginAndGetTokensWithUser(t, pool, cfg)

		// Victim resets their password
		rawToken := createResetToken(t, userID, time.Now().Add(30*time.Minute))
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token": rawToken, "new_password": "BrandNewPass1!",
		})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// The pre-reset refresh token must be dead
		w = doRequest(r, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": stolenRefresh,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code,
			"refresh token survived a password reset — compromised sessions outlive the reset")
	})

	t.Run("reset token is single use — second attempt returns 400", func(t *testing.T) {
		cleanAuthTables(t, pool)
		userID, _ := createTestUser(t, pool, "OldPassword1!", models.RoleAttendee)
		rawToken := createResetToken(t, userID, time.Now().Add(30*time.Minute))

		r := buildAuthRouter(pool, cfg)
		body := map[string]string{"token": rawToken, "new_password": "NewPassword2@"}
		require.Equal(t, http.StatusOK, doRequest(r, http.MethodPost, "/auth/reset_password", body).Code)

		body["new_password"] = "AnotherPass3#"
		assert.Equal(t, http.StatusBadRequest,
			doRequest(r, http.MethodPost, "/auth/reset_password", body).Code,
			"used reset token was accepted a second time")
	})

	t.Run("expired reset token returns 400", func(t *testing.T) {
		cleanAuthTables(t, pool)
		userID, _ := createTestUser(t, pool, "OldPassword1!", models.RoleAttendee)
		rawToken := createResetToken(t, userID, time.Now().Add(-1*time.Minute))

		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token": rawToken, "new_password": "NewPassword2@",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("reset with invalid UUID token returns 400", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token": "not-a-uuid", "new_password": "NewSecurePass1!",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("reset with well-formed but non-existent token returns 400", func(t *testing.T) {
		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token": "550e8400-e29b-41d4-a716-446655440000", "new_password": "NewSecurePass1!",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("weak new password returns 400 and does not consume the token", func(t *testing.T) {
		cleanAuthTables(t, pool)
		userID, _ := createTestUser(t, pool, "OldPassword1!", models.RoleAttendee)
		rawToken := createResetToken(t, userID, time.Now().Add(30*time.Minute))

		r := buildAuthRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token": rawToken, "new_password": "weak",
		})
		require.Equal(t, http.StatusBadRequest, w.Code)

		// The token must still work with a strong password
		w = doRequest(r, http.MethodPost, "/auth/reset_password", map[string]string{
			"token": rawToken, "new_password": "StrongEnough1!",
		})
		assert.Equal(t, http.StatusOK, w.Code, "token was burned by a failed validation attempt")
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// JWT / role middleware
// ═══════════════════════════════════════════════════════════════════════════

func TestAuthMiddleware(t *testing.T) {
	pool := setupAuthTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", middleware.AuthMiddleware(cfg), handlers.TestProtectedHandler())
	r.GET("/organizer", middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware(),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/admin", middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware(),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/optional", middleware.OptionalAuth(cfg), func(c *gin.Context) {
		_, loggedIn := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{"logged_in": loggedIn})
	})
	r.POST("/auth/login", handlers.LoginHandler(pool, cfg))

	get := func(path, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	freshClaims := func(role string) jwt.MapClaims {
		return jwt.MapClaims{
			"user_id": "11111111-1111-1111-1111-111111111111",
			"role":    role,
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		}
	}

	t.Run("no Authorization header returns 401", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, get("/protected", "").Code)
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
		assert.Equal(t, http.StatusOK, get("/protected", accessToken).Code)
	})

	t.Run("tampered JWT signature returns 401", func(t *testing.T) {
		cleanAuthTables(t, pool)
		accessToken, _ := loginAndGetTokens(t, pool, cfg)
		tampered := accessToken[:len(accessToken)-4] + "xxxx"
		assert.Equal(t, http.StatusUnauthorized, get("/protected", tampered).Code)
	})

	t.Run("expired JWT returns 401", func(t *testing.T) {
		claims := freshClaims("attendee")
		claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
		expired := mintJWT(t, cfg.JWTSecret, claims)
		assert.Equal(t, http.StatusUnauthorized, get("/protected", expired).Code)
	})

	t.Run("JWT signed with a DIFFERENT secret returns 401", func(t *testing.T) {
		forged := mintJWT(t, "attacker-controlled-secret-32-chars!!", freshClaims("admin"))
		assert.Equal(t, http.StatusUnauthorized, get("/protected", forged).Code)
	})

	t.Run("alg confusion — unsigned 'none' token returns 401", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodNone, freshClaims("admin"))
		unsigned, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, get("/protected", unsigned).Code,
			"'alg: none' token accepted — algorithm check is broken")
	})

	t.Run("JWT missing user_id claim returns 401", func(t *testing.T) {
		claims := freshClaims("attendee")
		delete(claims, "user_id")
		tok := mintJWT(t, cfg.JWTSecret, claims)
		assert.Equal(t, http.StatusUnauthorized, get("/protected", tok).Code)
	})

	t.Run("JWT missing role claim returns 401", func(t *testing.T) {
		claims := freshClaims("attendee")
		delete(claims, "role")
		tok := mintJWT(t, cfg.JWTSecret, claims)
		assert.Equal(t, http.StatusUnauthorized, get("/protected", tok).Code)
	})

	t.Run("role gates — attendee blocked from organizer/admin routes", func(t *testing.T) {
		attendee := mintJWT(t, cfg.JWTSecret, freshClaims(string(models.RoleAttendee)))
		assert.Equal(t, http.StatusForbidden, get("/organizer", attendee).Code)
		assert.Equal(t, http.StatusForbidden, get("/admin", attendee).Code)
	})

	t.Run("role gates — organizer allowed on organizer route, blocked from admin", func(t *testing.T) {
		organizer := mintJWT(t, cfg.JWTSecret, freshClaims(string(models.RoleOrganizer)))
		assert.Equal(t, http.StatusOK, get("/organizer", organizer).Code)
		assert.Equal(t, http.StatusForbidden, get("/admin", organizer).Code)
	})

	t.Run("role gates — admin allowed everywhere", func(t *testing.T) {
		admin := mintJWT(t, cfg.JWTSecret, freshClaims(string(models.RoleAdmin)))
		assert.Equal(t, http.StatusOK, get("/organizer", admin).Code)
		assert.Equal(t, http.StatusOK, get("/admin", admin).Code)
	})

	t.Run("OptionalAuth — invalid token continues as guest, valid token identifies user", func(t *testing.T) {
		// Garbage token → still 200, but as guest
		w := get("/optional", "garbage.token.here")
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, false, resp["logged_in"])

		// Valid token → logged in
		valid := mintJWT(t, cfg.JWTSecret, freshClaims("attendee"))
		w = get("/optional", valid)
		require.Equal(t, http.StatusOK, w.Code)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["logged_in"])
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// FIX 4 — auth rate limiting (in-memory fallback, no Redis required)
// ═══════════════════════════════════════════════════════════════════════════
//
// These tests pass a nil *ratelimit.Manager, exercising exactly the code path
// that runs in production when Redis is down. Before the fix, that scenario
// left the auth endpoints completely unlimited.

func TestAuthRateLimiting(t *testing.T) {
	gin.SetMode(gin.TestMode)

	okHandler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }

	t.Run("RateLimitAuthByIP with nil manager — enforces limit in-memory", func(t *testing.T) {
		r := gin.New()
		r.POST("/limited", middleware.RateLimitAuthByIP(nil,
			ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 600.0, TTL: time.Hour},
			"test-ip"), okHandler)

		// httptest requests share the same RemoteAddr → same IP bucket.
		for i := 1; i <= 3; i++ {
			w := doRequest(r, http.MethodPost, "/limited", nil)
			assert.Equalf(t, http.StatusOK, w.Code, "request %d should be allowed", i)
		}

		w := doRequest(r, http.MethodPost, "/limited", nil)
		assert.Equal(t, http.StatusTooManyRequests, w.Code,
			"4th request must be blocked — nil-manager fallback is not limiting")
		assert.NotEmpty(t, w.Header().Get("Retry-After"), "429 must carry a Retry-After header")
	})

	t.Run("RateLimitLoginByEmail with nil manager — blocks per-account brute force", func(t *testing.T) {
		r := gin.New()
		r.POST("/login", middleware.RateLimitLoginByEmail(nil,
			ratelimit.BucketConfig{Capacity: 3, RefillRate: 1.0 / 600.0, TTL: time.Hour},
			"test-email"), okHandler)

		target := map[string]string{"email": "victim@test.com", "password": "Guess1!"}
		for i := 1; i <= 3; i++ {
			w := doRequest(r, http.MethodPost, "/login", target)
			assert.Equalf(t, http.StatusOK, w.Code, "attempt %d on victim should pass", i)
		}

		// 4th attempt on the same account is blocked...
		w := doRequest(r, http.MethodPost, "/login", target)
		assert.Equal(t, http.StatusTooManyRequests, w.Code,
			"per-account bucket did not trip — single account can be brute-forced")

		// ...but a different account is unaffected.
		w = doRequest(r, http.MethodPost, "/login", map[string]string{
			"email": "someone-else@test.com", "password": "Whatever1!",
		})
		assert.Equal(t, http.StatusOK, w.Code, "unrelated account was collaterally blocked")
	})

	t.Run("email bucket is case/whitespace-insensitive (no bypass via ViCtIm@...)", func(t *testing.T) {
		r := gin.New()
		r.POST("/login", middleware.RateLimitLoginByEmail(nil,
			ratelimit.BucketConfig{Capacity: 2, RefillRate: 1.0 / 600.0, TTL: time.Hour},
			"test-email-norm"), okHandler)

		variants := []string{"victim2@test.com", "VICTIM2@test.com", "  Victim2@Test.com  "}
		codes := []int{}
		for _, v := range variants {
			w := doRequest(r, http.MethodPost, "/login", map[string]string{"email": v, "password": "x1234567!"})
			codes = append(codes, w.Code)
		}
		assert.Equal(t, http.StatusOK, codes[0])
		assert.Equal(t, http.StatusOK, codes[1])
		assert.Equal(t, http.StatusTooManyRequests, codes[2],
			"case/whitespace variants bypassed the per-account bucket")
	})

	t.Run("body is preserved for the downstream handler after email peek", func(t *testing.T) {
		r := gin.New()
		r.POST("/echo", middleware.RateLimitLoginByEmail(nil,
			ratelimit.BucketConfig{Capacity: 100, RefillRate: 1, TTL: time.Hour},
			"test-echo"), func(c *gin.Context) {
			var in struct {
				Email string `json:"email" binding:"required"`
			}
			if err := c.ShouldBindJSON(&in); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "body was consumed by middleware"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"email": in.Email})
		})

		w := doRequest(r, http.MethodPost, "/echo", map[string]string{"email": "reader@test.com"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "reader@test.com", resp["email"], "handler saw a different body than the client sent")
	})

	t.Run("malformed body passes through to the handler (which 400s it)", func(t *testing.T) {
		r := gin.New()
		r.POST("/login", middleware.RateLimitLoginByEmail(nil,
			ratelimit.BucketConfig{Capacity: 3, RefillRate: 1, TTL: time.Hour},
			"test-malformed"), func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad body"})
		})

		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("{not json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "malformed body should reach the handler, not 500")
	})

	t.Run("tokens refill over time", func(t *testing.T) {
		r := gin.New()
		// 1-token bucket refilling 10 tokens/sec → empty for ~100ms after use
		r.POST("/refill", middleware.RateLimitAuthByIP(nil,
			ratelimit.BucketConfig{Capacity: 1, RefillRate: 10, TTL: time.Hour},
			"test-refill"), okHandler)

		require.Equal(t, http.StatusOK, doRequest(r, http.MethodPost, "/refill", nil).Code)
		require.Equal(t, http.StatusTooManyRequests, doRequest(r, http.MethodPost, "/refill", nil).Code)

		time.Sleep(150 * time.Millisecond)
		assert.Equal(t, http.StatusOK, doRequest(r, http.MethodPost, "/refill", nil).Code,
			"bucket did not refill")
	})
}