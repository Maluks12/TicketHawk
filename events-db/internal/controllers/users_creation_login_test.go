package controllers_test

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"math/rand"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	//"time"

// 	"events-db/internal/config"
// 	//"events-db/internal/controllers"
// 	"events-db/internal/handlers"
// 	"events-db/internal/models"
// 	"events-db/internal/repository"
// 	//"events-db/internal/utils"

// 	"github.com/gin-gonic/gin"
// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"golang.org/x/crypto/bcrypt"
// )

// // setupTestDB connects to your Dockerized test database
// func setupTestDB(t *testing.T) *pgxpool.Pool {
// 	dbURL := "postgresql://testuser:testpass@localhost:5433/test_db" // update with your Docker test DB
// 	pool, err := pgxpool.New(context.Background(), dbURL)
// 	if err != nil {
// 		t.Fatalf("Failed to connect to test database: %v", err)
// 	}

// 	// Clean tables before tests
// 	_, err = pool.Exec(context.Background(), "TRUNCATE users CASCADE")
// 	if err != nil {
// 		t.Fatalf("Failed to truncate users table: %v", err)
// 	}

// 	return pool
// }

// // randomEmail generates a random email to avoid duplicates
// func randomEmail() string {
// 	return fmt.Sprintf("testuser%d@example.com", rand.Intn(100000))
// }

// // TestUserRegistrationAndLogin tests CreateUserHandler and LoginHandler
// func TestUserRegistrationAndLogin(t *testing.T) {
// 	gin.SetMode(gin.TestMode)
// 	pool := setupTestDB(t)
// 	defer pool.Close()

// 	cfg := &config.Config{
// 		JWTSecret: "testsecret123", // dummy JWT secret for testing
// 	}

// 	t.Run("Create_user_successfully", func(t *testing.T) {
// 		email := randomEmail()
// 		password := "password123"

// 		reqBody := map[string]string{
// 			"email":    email,
// 			"password": password,
// 		}
// 		bodyBytes, _ := json.Marshal(reqBody)

// 		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(bodyBytes))
// 		req.Header.Set("Content-Type", "application/json")
// 		w := httptest.NewRecorder()

// 		router := gin.Default()
// 		router.POST("/register", handlers.CreateUserhandler(pool))
// 		router.ServeHTTP(w, req)

// 		if w.Code != http.StatusCreated {
// 			t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
// 		}

// 		var createdUser models.User
// 		if err := json.Unmarshal(w.Body.Bytes(), &createdUser); err != nil {
// 			t.Fatalf("failed to unmarshal response: %v", err)
// 		}

// 		if createdUser.Email != email {
// 			t.Errorf("expected email %s, got %s", email, createdUser.Email)
// 		}
// 	})

// 	t.Run("Fail_to_create_duplicate_user", func(t *testing.T) {
// 		email := randomEmail()
// 		password := "password123"

// 		// First create the user
// 		user := &models.User{
// 			Email:    email,
// 			Password: password,
// 			Role:     "user",
// 		}
// 		_, err := repository.CreateUser(pool, user)
// 		if err != nil {
// 			t.Fatalf("failed to create initial user: %v", err)
// 		}

// 		// Attempt to create the same email via handler
// 		reqBody := map[string]string{
// 			"email":    email,
// 			"password": password,
// 		}
// 		bodyBytes, _ := json.Marshal(reqBody)

// 		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(bodyBytes))
// 		req.Header.Set("Content-Type", "application/json")
// 		w := httptest.NewRecorder()

// 		router := gin.Default()
// 		router.POST("/register", handlers.CreateUserhandler(pool))
// 		router.ServeHTTP(w, req)

// 		if w.Code != http.StatusBadRequest {
// 			t.Errorf("expected 400 for duplicate, got %d, body: %s", w.Code, w.Body.String())
// 		}
// 	})

// 	t.Run("Login_successfully_returns_JWT_with_claims", func(t *testing.T) {
// 		email := randomEmail()
// 		password := "password123"

// 		// First create the user
// 		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
// 		user := &models.User{
// 			Email:    email,
// 			Password: string(hashedPassword),
// 			Role:     "user",
// 		}
// 		_, err := repository.CreateUser(pool, user)
// 		if err != nil {
// 			t.Fatalf("failed to create user: %v", err)
// 		}

// 		// Login
// 		reqBody := map[string]string{
// 			"email":    email,
// 			"password": password,
// 		}
// 		bodyBytes, _ := json.Marshal(reqBody)

// 		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(bodyBytes))
// 		req.Header.Set("Content-Type", "application/json")
// 		w := httptest.NewRecorder()

// 		router := gin.Default()
// 		router.POST("/login", handlers.LoginHandler(pool,cfg))
// 		router.ServeHTTP(w, req)

// 		if w.Code != http.StatusOK {
// 			t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
// 		}

// 		var resp struct {
// 			Token string `json:"token"`
// 		}
// 		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
// 			t.Fatalf("failed to unmarshal login response: %v", err)
// 		}

// 		if resp.Token == "" {
// 			t.Errorf("expected JWT token, got empty string")
// 		}
// 	})
// }