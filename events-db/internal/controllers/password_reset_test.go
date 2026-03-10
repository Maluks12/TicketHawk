package controllers

// import (
// 	"bytes"
// 	"context"
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"encoding/json"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	"time"

// 	"events-db/internal/handlers"
// 	"events-db/internal/models"
// 	"events-db/internal/repository"
// 	"events-db/internal/utils"

// 	"github.com/gin-gonic/gin"
// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"golang.org/x/crypto/bcrypt"
// )

// // ==========================================
// // Test Database Setup - SIMPLIFIED VERSION
// // ==========================================

// func setupTestDB(t *testing.T) *pgxpool.Pool {
// 	// Connect to test database
// 	dbURL := "postgresql://testuser:testpass@localhost:5433/test_db"
	
// 	pool, err := pgxpool.New(context.Background(), dbURL)
// 	if err != nil {
// 		t.Fatalf("Failed to connect to test database: %v\nMake sure Docker container is running:\n  docker run -d --name test-postgres -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=test_db -p 5433:5432 postgres:15", err)
// 	}

// 	// Create tables directly (no migration files needed!)
// 	createTestTables(t, pool)
	
// 	// Clean existing data
// 	cleanupTestData(pool)

// 	return pool
// }

// func createTestTables(t *testing.T, pool *pgxpool.Pool) {
// 	// Create users table
// 	_, err := pool.Exec(context.Background(), `
// 		CREATE TABLE IF NOT EXISTS users (
// 			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			email VARCHAR(255) UNIQUE NOT NULL,
// 			password TEXT NOT NULL,
// 			role VARCHAR(50) DEFAULT 'user',
// 			created_at TIMESTAMP DEFAULT NOW(),
// 			updated_at TIMESTAMP DEFAULT NOW()
// 		)
// 	`)
// 	if err != nil {
// 		t.Logf("Warning: Could not create users table: %v", err)
// 	}

// 	// Create password_resets table
// 	_, err = pool.Exec(context.Background(), `
// 		CREATE TABLE IF NOT EXISTS password_resets (
// 			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
// 			token_hash TEXT UNIQUE NOT NULL,
// 			expires_at TIMESTAMP NOT NULL,
// 			created_at TIMESTAMP DEFAULT NOW()
// 		)
// 	`)
// 	if err != nil {
// 		t.Logf("Warning: Could not create password_resets table: %v", err)
// 	}
// }

// func cleanupTestDB(pool *pgxpool.Pool) {
// 	cleanupTestData(pool)
// 	pool.Close()
// }

// func cleanupTestData(pool *pgxpool.Pool) {
// 	// Clean tables in reverse order (respecting foreign keys)
// 	pool.Exec(context.Background(), "TRUNCATE TABLE password_resets CASCADE")
// 	pool.Exec(context.Background(), "TRUNCATE TABLE users CASCADE")
// }

// // ==========================================
// // Helper Functions
// // ==========================================

// func createTestUser(t *testing.T, pool *pgxpool.Pool, email, password string) *models.User {
// 	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
// 	if err != nil {
// 		t.Fatalf("Failed to hash password: %v", err)
// 	}

// 	user := &models.User{
// 		Email:    email,
// 		Password: string(hashedPassword),
// 		Role:     "user",
// 	}

// 	createdUser, err := repository.CreateUser(pool, user)
// 	if err != nil {
// 		t.Fatalf("Failed to create test user: %v", err)
// 	}

// 	return createdUser
// }

// func makeRequest(method, url string, body interface{}, handler gin.HandlerFunc) *httptest.ResponseRecorder {
// 	jsonBody, _ := json.Marshal(body)

// 	req := httptest.NewRequest(method, url, bytes.NewBuffer(jsonBody))
// 	req.Header.Set("Content-Type", "application/json")

// 	w := httptest.NewRecorder()

// 	gin.SetMode(gin.TestMode)
// 	c, _ := gin.CreateTestContext(w)
// 	c.Request = req

// 	handler(c)

// 	return w
// }

// func parseResponse(w *httptest.ResponseRecorder) map[string]interface{} {
// 	var response map[string]interface{}
// 	json.Unmarshal(w.Body.Bytes(), &response)
// 	return response
// }

// // ==========================================
// // Actual Tests
// // ==========================================

// func TestRequestPasswordResetHandler(t *testing.T) {
// 	pool := setupTestDB(t)
// 	defer cleanupTestDB(pool)

// 	// Create test user
// 	createTestUser(t, pool, "test@example.com", "password123")

// 	tests := []struct {
// 		name            string
// 		email           string
// 		expectedStatus  int
// 		shouldHaveError bool
// 	}{
// 		{
// 			name:            "Valid email - user exists",
// 			email:           "test@example.com",
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name:            "Valid email - user does not exist",
// 			email:           "nonexistent@example.com",
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name:            "Invalid email format",
// 			email:           "not-an-email",
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 		{
// 			name:            "Empty email",
// 			email:           "",
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			requestBody := map[string]string{"email": tt.email}
// 			w := makeRequest("POST", "/api/password-reset/request", requestBody, handlers.RequestPasswordResetHandler(pool))

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseResponse(w)

// 			if tt.shouldHaveError {
// 				assert.Contains(t, response, "error")
// 			} else {
// 				assert.Contains(t, response, "message")
// 			}
// 		})
// 	}
// }

// func TestResetPasswordHandler(t *testing.T) {
// 	pool := setupTestDB(t)
// 	defer cleanupTestDB(pool)

// 	// Create test user
// 	testUser := createTestUser(t, pool, "test@example.com", "oldpassword123")

// 	// Verify user was created
// 	if testUser == nil {
// 		t.Fatal("Test user was not created")
// 	}

// 	tests := []struct {
// 		name            string
// 		setupToken      bool   // Whether to create valid reset token
// 		token           string
// 		newPassword     string
// 		expectedStatus  int
// 		shouldHaveError bool
// 	}{
// 		{
// 			name:            "Valid token and password",
// 			setupToken:      true,
// 			token:           "", // Will be set in test
// 			newPassword:     "NewSecurePass123!",
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name:            "Invalid token",
// 			setupToken:      false,
// 			token:           "invalid-token-12345",
// 			newPassword:     "NewSecurePass123!",
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 		{
// 			name:            "Password too short",
// 			setupToken:      true,
// 			token:           "", // Will be set in test
// 			newPassword:     "short",
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// Clean up password resets before each test
// 			pool.Exec(context.Background(), "TRUNCATE TABLE password_resets CASCADE")

// 			var rawToken string

// 			// Setup valid token if needed
// 			if tt.setupToken {
// 				var hashedToken string
// 				var err error
				
// 				rawToken, hashedToken, err = utils.GenerateResetToken()
// 				if err != nil {
// 					t.Fatalf("Failed to generate reset token: %v", err)
// 				}

// 				expiresAt := time.Now().Add(30 * time.Minute)

// 				err = repository.CreatePasswordReset(pool, testUser.ID, hashedToken, expiresAt)
// 				if err != nil {
// 					t.Fatalf("Failed to create password reset: %v", err)
// 				}

// 				tt.token = rawToken
// 			}

// 			// Make request
// 			requestBody := map[string]string{
// 				"token":        tt.token,
// 				"new_password": tt.newPassword,
// 			}
// 			w := makeRequest("POST", "/api/password-reset/reset", requestBody, handlers.ResetPasswordHandler(pool))

// 			// Assert status code
// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			// Parse response
// 			response := parseResponse(w)

// 			if tt.shouldHaveError {
// 				assert.Contains(t, response, "error")
// 			} else {
// 				assert.Contains(t, response, "message")
// 				assert.Equal(t, "password reset successful", response["message"])
// 			}
// 		})
// 	}
// }

// // Test that password is actually updated
// func TestResetPasswordHandler_PasswordUpdated(t *testing.T) {
// 	pool := setupTestDB(t)
// 	defer cleanupTestDB(pool)

// 	// Create test user
// 	testUser := createTestUser(t, pool, "test@example.com", "oldpassword123")

// 	// Create valid reset token
// 	rawToken, hashedToken, err := utils.GenerateResetToken()
// 	if err != nil {
// 		t.Fatalf("Failed to generate reset token: %v", err)
// 	}

// 	expiresAt := time.Now().Add(30 * time.Minute)
// 	err = repository.CreatePasswordReset(pool, testUser.ID, hashedToken, expiresAt)
// 	if err != nil {
// 		t.Fatalf("Failed to create password reset: %v", err)
// 	}

// 	// Reset password
// 	requestBody := map[string]string{
// 		"token":        rawToken,
// 		"new_password": "NewSecurePass123!",
// 	}
// 	w := makeRequest("POST", "/api/password-reset/reset", requestBody, handlers.ResetPasswordHandler(pool))

// 	// Assert success
// 	assert.Equal(t, http.StatusOK, w.Code)

// 	// Get updated user from database
// 	updatedUser, err := repository.GetUserByID(pool, testUser.ID)
// 	if err != nil {
// 		t.Fatalf("Failed to get updated user: %v", err)
// 	}

// 	// Verify new password works
// 	err = bcrypt.CompareHashAndPassword([]byte(updatedUser.Password), []byte("NewSecurePass123!"))
// 	assert.NoError(t, err, "New password should match")

// 	// Verify old password doesn't work
// 	err = bcrypt.CompareHashAndPassword([]byte(updatedUser.Password), []byte("oldpassword123"))
// 	assert.Error(t, err, "Old password should not match")
// }

// func TestTokenHashingDebug(t *testing.T) {
// 	pool := setupTestDB(t)
// 	defer cleanupTestDB(pool)

// 	// Create test user
// 	testUser := createTestUser(t, pool, "test@example.com", "password123")

// 	// Generate token
// 	rawToken, hashedToken, err := utils.GenerateResetToken()
// 	if err != nil {
// 		t.Fatalf("Failed to generate token: %v", err)
// 	}

// 	t.Logf("Raw token: %s", rawToken)
// 	t.Logf("Hashed token (from GenerateResetToken): %s", hashedToken)

// 	// Store in database
// 	expiresAt := time.Now().Add(30 * time.Minute)
// 	err = repository.CreatePasswordReset(pool, testUser.ID, hashedToken, expiresAt)
// 	if err != nil {
// 		t.Fatalf("Failed to create password reset: %v", err)
// 	}

// 	// Now hash the raw token the same way the handler does
// 	hash := sha256.Sum256([]byte(rawToken))
// 	tokenHashFromHandler := hex.EncodeToString(hash[:])
	
// 	t.Logf("Hashed token (from handler logic): %s", tokenHashFromHandler)

// 	// Check if they match
// 	if hashedToken != tokenHashFromHandler {
// 		t.Errorf("TOKEN MISMATCH!\nStored hash: %s\nHandler hash: %s", hashedToken, tokenHashFromHandler)
// 	}

// 	// Try to find token in database
// 	var count int
// 	err = pool.QueryRow(context.Background(), 
// 		"SELECT COUNT(*) FROM password_resets WHERE token_hash = $1",
// 		hashedToken,
// 	).Scan(&count)

// 	t.Logf("Tokens found with stored hash: %d", count)

// 	err = pool.QueryRow(context.Background(), 
// 		"SELECT COUNT(*) FROM password_resets WHERE token_hash = $1",
// 		tokenHashFromHandler,
// 	).Scan(&count)

// 	t.Logf("Tokens found with handler hash: %d", count)
// }