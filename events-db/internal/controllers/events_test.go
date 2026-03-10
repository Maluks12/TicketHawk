package controllers

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	"time"

// 	"events-db/internal/handlers"
// 	"events-db/internal/models"
// 	"events-db/internal/repository"

// 	//"events-db/internal/utils"

// 	"github.com/gin-gonic/gin"
// 	"github.com/google/uuid"
// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"golang.org/x/crypto/bcrypt"
// )

// // ==========================================
// // Test Database Setup for Events
// // ==========================================

// func setupEventsTestDB(t *testing.T) *pgxpool.Pool {
// 	dbURL := "postgresql://testuser:testpass@localhost:5433/test_db"

// 	pool, err := pgxpool.New(context.Background(), dbURL)
// 	if err != nil {
// 		t.Fatalf("Failed to connect to test database: %v\nMake sure Docker container is running", err)
// 	}

// 	createEventsTestTables(t, pool)
// 	cleanupEventsTestData(pool)

// 	return pool
// }

// func createEventsTestTables(t *testing.T, pool *pgxpool.Pool) {
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

// 	// Create categories table
// 	_, err = pool.Exec(context.Background(), `
// 		CREATE TABLE IF NOT EXISTS categories (
// 			id SERIAL PRIMARY KEY,
// 			name VARCHAR(100) UNIQUE NOT NULL,
// 			created_at TIMESTAMP DEFAULT NOW()
// 		)
// 	`)
// 	if err != nil {
// 		t.Logf("Warning: Could not create categories table: %v", err)
// 	}

// 	// Create events table
// 	_, err = pool.Exec(context.Background(), `
// 		CREATE TABLE IF NOT EXISTS events (
// 			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			title VARCHAR(200) NOT NULL,
// 			descript TEXT NOT NULL,
// 			start_time TIMESTAMP NOT NULL,
// 			end_time TIMESTAMP NOT NULL,
// 			cover_image_url TEXT,
// 			views INT DEFAULT 0,
// 			is_featured BOOLEAN DEFAULT FALSE,
// 			expires_at TIMESTAMP NOT NULL,
// 			venue_name VARCHAR(200) NOT NULL,
// 			city VARCHAR(100) NOT NULL,
// 			address TEXT NOT NULL,
// 			category_id INT REFERENCES categories(id),
// 			featured_until TIMESTAMP,
// 			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
// 			created_at TIMESTAMP DEFAULT NOW(),
// 			updated_at TIMESTAMP DEFAULT NOW()
// 		)
// 	`)
// 	if err != nil {
// 		t.Logf("Warning: Could not create events table: %v", err)
// 	}

// 	// Create search_vector column for full-text search
// 	_, err = pool.Exec(context.Background(), `
// 		ALTER TABLE events ADD COLUMN IF NOT EXISTS search_vector tsvector
// 	`)
// 	if err != nil {
// 		t.Logf("Warning: Could not add search_vector column: %v", err)
// 	}

// 	// Create index for search
// 	_, err = pool.Exec(context.Background(), `
// 		CREATE INDEX IF NOT EXISTS idx_events_search_vector ON events USING gin(search_vector)
// 	`)
// 	if err != nil {
// 		t.Logf("Warning: Could not create search index: %v", err)
// 	}
// }

// func cleanupEventsTestDB(pool *pgxpool.Pool) {
// 	cleanupEventsTestData(pool)
// 	pool.Close()
// }

// func cleanupEventsTestData(pool *pgxpool.Pool) {
// 	pool.Exec(context.Background(), "TRUNCATE TABLE events CASCADE")
// 	pool.Exec(context.Background(), "TRUNCATE TABLE categories RESTART IDENTITY CASCADE")
// 	pool.Exec(context.Background(), "TRUNCATE TABLE users CASCADE")
// }

// // ==========================================
// // Helper Functions
// // ==========================================

// func createEventsTestUser(t *testing.T, pool *pgxpool.Pool, email, password, role string) *models.User {
// 	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
// 	if err != nil {
// 		t.Fatalf("Failed to hash password: %v", err)
// 	}

// 	user := &models.User{
// 		Email:    email,
// 		Password: string(hashedPassword),
// 		Role:     role,
// 	}

// 	createdUser, err := repository.CreateUser(pool, user)
// 	if err != nil {
// 		t.Fatalf("Failed to create test user: %v", err)
// 	}

// 	return createdUser
// }

// func createTestCategory(t *testing.T, pool *pgxpool.Pool, name string) int {
// 	var categoryID int
// 	err := pool.QueryRow(context.Background(),
// 		"INSERT INTO categories (name) VALUES ($1) RETURNING id",
// 		name,
// 	).Scan(&categoryID)

// 	if err != nil {
// 		t.Fatalf("Failed to create test category: %v", err)
// 	}

// 	return categoryID
// }

// func createTestEvent(t *testing.T, pool *pgxpool.Pool, userID string, categoryID int, title string) *models.Events {
// 	now := time.Now()
// 	event, err := repository.CreateEvents(
// 		pool,
// 		title,
// 		"Test event description",
// 		now.Add(24*time.Hour),   // Start tomorrow
// 		now.Add(48*time.Hour),   // End in 2 days
// 		"https://example.com/image.jpg",
// 		0,
// 		false,
// 		now.Add(72*time.Hour),  // Expires in 3 days
// 		"Test Venue",
// 		"Test City",
// 		"123 Test St",
// 		categoryID,
// 		nil,
// 		userID,
// 	)

// 	if err != nil {
// 		t.Fatalf("Failed to create test event: %v", err)
// 	}

// 	return event
// }

// func makeAuthenticatedRequest(method, url string, body interface{}, userID string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
// 	jsonBody, _ := json.Marshal(body)

// 	req := httptest.NewRequest(method, url, bytes.NewBuffer(jsonBody))
// 	req.Header.Set("Content-Type", "application/json")

// 	w := httptest.NewRecorder()

// 	gin.SetMode(gin.TestMode)
// 	c, _ := gin.CreateTestContext(w)
// 	c.Request = req
	
// 	// Set user_id in context (simulating auth middleware)
// 	c.Set("user_id", userID)

// 	handler(c)

// 	return w
// }

// // Helper for requests that need URL params
// func makeAuthenticatedRequestWithParams(method, url string, body interface{}, userID string, handler gin.HandlerFunc, params gin.Params) *httptest.ResponseRecorder {
// 	jsonBody, _ := json.Marshal(body)

// 	req := httptest.NewRequest(method, url, bytes.NewBuffer(jsonBody))
// 	req.Header.Set("Content-Type", "application/json")

// 	w := httptest.NewRecorder()

// 	gin.SetMode(gin.TestMode)
// 	c, _ := gin.CreateTestContext(w)
// 	c.Request = req
	
// 	// Set user_id in context (simulating auth middleware)
// 	c.Set("user_id", userID)
	
// 	// Set URL params
// 	c.Params = params

// 	handler(c)

// 	return w
// }

// func parseEventsResponse(w *httptest.ResponseRecorder) map[string]interface{} {
// 	var response map[string]interface{}
// 	json.Unmarshal(w.Body.Bytes(), &response)
// 	return response
// }

// // ==========================================
// // Create Event Tests
// // ==========================================

// func TestCreateEventHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	_ = createTestCategory(t, pool, "Music")

// 	tests := []struct {
// 		name            string
// 		requestBody     map[string]interface{}
// 		expectedStatus  int
// 		shouldHaveError bool
// 	}{
// 		{
// 			name: "Valid event creation",
// 			requestBody: map[string]interface{}{
// 				"title":           "Test Concert",
// 				"descript":        "A great music event",
// 				"start_time":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
// 				"end_time":        time.Now().Add(48 * time.Hour).Format(time.RFC3339),
// 				"cover_image_url": "https://example.com/image.jpg",
// 				"views":           0,
// 				"is_featured":     false,
// 				"expires_at":      time.Now().Add(72 * time.Hour).Format(time.RFC3339),
// 				"venue_name":      "Madison Square Garden",
// 				"city":            "New York",
// 				"address":         "4 Pennsylvania Plaza",
// 				"category":        "Music",
// 			},
// 			expectedStatus:  http.StatusCreated,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name: "Featured event with featured_until",
// 			requestBody: map[string]interface{}{
// 				"title":           "Featured Concert",
// 				"descript":        "A featured music event",
// 				"start_time":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
// 				"end_time":        time.Now().Add(48 * time.Hour).Format(time.RFC3339),
// 				"cover_image_url": "https://example.com/image.jpg",
// 				"views":           0,
// 				"is_featured":     true,
// 				"featured_until":  time.Now().Add(120 * time.Hour).Format(time.RFC3339),
// 				"expires_at":      time.Now().Add(72 * time.Hour).Format(time.RFC3339),
// 				"venue_name":      "Madison Square Garden",
// 				"city":            "New York",
// 				"address":         "4 Pennsylvania Plaza",
// 				"category":        "Music",
// 			},
// 			expectedStatus:  http.StatusCreated,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name: "Featured event without featured_until - should fail",
// 			requestBody: map[string]interface{}{
// 				"title":           "Featured Concert",
// 				"descript":        "A featured music event",
// 				"start_time":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
// 				"end_time":        time.Now().Add(48 * time.Hour).Format(time.RFC3339),
// 				"cover_image_url": "https://example.com/image.jpg",
// 				"views":           0,
// 				"is_featured":     true,
// 				"expires_at":      time.Now().Add(72 * time.Hour).Format(time.RFC3339),
// 				"venue_name":      "Madison Square Garden",
// 				"city":            "New York",
// 				"address":         "4 Pennsylvania Plaza",
// 				"category":        "Music",
// 			},
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 		{
// 			name: "Invalid category",
// 			requestBody: map[string]interface{}{
// 				"title":           "Test Concert",
// 				"descript":        "A great music event",
// 				"start_time":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
// 				"end_time":        time.Now().Add(48 * time.Hour).Format(time.RFC3339),
// 				"cover_image_url": "https://example.com/image.jpg",
// 				"views":           0,
// 				"is_featured":     false,
// 				"expires_at":      time.Now().Add(72 * time.Hour).Format(time.RFC3339),
// 				"venue_name":      "Madison Square Garden",
// 				"city":            "New York",
// 				"address":         "4 Pennsylvania Plaza",
// 				"category":        "InvalidCategory",
// 			},
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 		{
// 			name: "Missing required fields",
// 			requestBody: map[string]interface{}{
// 				"title": "Test Concert",
// 			},
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			w := makeAuthenticatedRequest("POST", "/api/events", tt.requestBody, user.ID, handlers.CreateEventHandlers(pool))

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseEventsResponse(w)

// 			if tt.shouldHaveError {
// 				assert.Contains(t, response, "error")
// 			} else {
// 				assert.Contains(t, response, "id")
// 				assert.Equal(t, tt.requestBody["title"], response["title"])
// 			}
// 		})
// 	}
// }

// // ==========================================
// // Get Event By ID Tests
// // ==========================================

// func TestGetEventByIDHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")
// 	event := createTestEvent(t, pool, user.ID, categoryID, "Test Event")

// 	tests := []struct {
// 		name            string
// 		eventID         string
// 		expectedStatus  int
// 		shouldHaveError bool
// 		checkViews      bool
// 	}{
// 		{
// 			name:            "Valid event ID",
// 			eventID:         event.ID,
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 			checkViews:      true,
// 		},
// 		{
// 			name:            "Invalid UUID format",
// 			eventID:         "not-a-uuid",
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 			checkViews:      false,
// 		},
// 		{
// 			name:            "Non-existent event",
// 			eventID:         uuid.New().String(),
// 			expectedStatus:  http.StatusNotFound,
// 			shouldHaveError: true,
// 			checkViews:      false,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// Use makeAuthenticatedRequestWithParams helper
// 			w := makeAuthenticatedRequestWithParams("GET", "/api/events/"+tt.eventID, nil, user.ID, 
// 				handlers.GetEventByIDHandlers(pool), gin.Params{{Key: "id", Value: tt.eventID}})

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseEventsResponse(w)

// 			if tt.shouldHaveError {
// 				assert.Contains(t, response, "error")
// 			} else {
// 				assert.Contains(t, response, "id")
// 				assert.Equal(t, event.ID, response["id"])
				
// 				if tt.checkViews {
// 					assert.NotNil(t, response["views"])
// 				}
// 			}
// 		})
// 	}
// }

// // ==========================================
// // Update Event Tests
// // ==========================================

// func TestUpdateEventHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	tests := []struct {
// 		name            string
// 		requestBody     map[string]interface{}
// 		expectedStatus  int
// 		shouldHaveError bool
// 	}{
// 		{
// 			name: "Valid update",
// 			requestBody: map[string]interface{}{
// 				"title": "Updated Title",
// 			},
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name: "Update multiple fields",
// 			requestBody: map[string]interface{}{
// 				"title":      "New Title",
// 				"descript":   "New Description",
// 				"venue_name": "New Venue",
// 			},
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name: "Featured event with featured_until",
// 			requestBody: map[string]interface{}{
// 				"is_featured":    true,
// 				"featured_until": time.Now().Add(120 * time.Hour).Format(time.RFC3339),
// 			},
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name: "Featured event without featured_until - should fail",
// 			requestBody: map[string]interface{}{
// 				"is_featured": true,
// 			},
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 		{
// 			name:            "No fields provided - should fail",
// 			requestBody:     map[string]interface{}{},
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 		{
// 			name: "Invalid UUID",
// 			requestBody: map[string]interface{}{
// 				"title": "Updated Title",
// 			},
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// Create fresh event for each test
// 			event := createTestEvent(t, pool, user.ID, categoryID, "Original Title")
			
// 			// Use special eventID for invalid UUID test
// 			eventID := event.ID
// 			if tt.name == "Invalid UUID" {
// 				eventID = "not-a-uuid"
// 			}

// 			w := makeAuthenticatedRequestWithParams("PUT", "/api/events/"+eventID, tt.requestBody, user.ID,
// 				handlers.UpdateEventHandlers(pool), gin.Params{{Key: "id", Value: eventID}})

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseEventsResponse(w)

// 			if tt.shouldHaveError {
// 				assert.Contains(t, response, "error")
// 			} else {
// 				assert.Contains(t, response, "id")
// 				if title, ok := tt.requestBody["title"]; ok {
// 					assert.Equal(t, title, response["title"])
// 				}
// 			}
			
// 			// Clean up
// 			pool.Exec(context.Background(), "DELETE FROM events WHERE id = $1", event.ID)
// 		})
// 	}
// }

// // ==========================================
// // Delete Event Tests
// // ==========================================

// func TestDeleteEventHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	otherUser := createEventsTestUser(t, pool, "other@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	tests := []struct {
// 		name            string
// 		setupEvent      bool
// 		userID          string
// 		otherUserEvent  bool
// 		expectedStatus  int
// 		shouldHaveError bool
// 	}{
// 		{
// 			name:            "Valid deletion",
// 			setupEvent:      true,
// 			userID:          "",  // Will use event owner
// 			otherUserEvent:  false,
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 		},
// 		{
// 			name:            "Delete non-existent event",
// 			setupEvent:      false,
// 			userID:          "",
// 			otherUserEvent:  false,
// 			expectedStatus:  http.StatusNotFound,
// 			shouldHaveError: true,
// 		},
// 		{
// 			name:            "Delete other user's event - should fail",
// 			setupEvent:      true,
// 			userID:          "",  // Will use different user
// 			otherUserEvent:  true,
// 			expectedStatus:  http.StatusNotFound,
// 			shouldHaveError: true,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			var eventID string
// 			var requestUserID string

// 			if tt.setupEvent {
// 				if tt.otherUserEvent {
// 					event := createTestEvent(t, pool, user.ID, categoryID, "Test Event")
// 					eventID = event.ID
// 					requestUserID = otherUser.ID  // Try to delete as different user
// 				} else {
// 					event := createTestEvent(t, pool, user.ID, categoryID, "Test Event")
// 					eventID = event.ID
// 					requestUserID = user.ID
// 				}
// 			} else {
// 				eventID = uuid.New().String()
// 				requestUserID = user.ID
// 			}

// 			w := makeAuthenticatedRequestWithParams("DELETE", "/api/events/"+eventID, nil, requestUserID,
// 				handlers.DeleteEventHandlers(pool), gin.Params{{Key: "id", Value: eventID}})

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseEventsResponse(w)

// 			if tt.shouldHaveError {
// 				assert.Contains(t, response, "error")
// 			} else {
// 				assert.Contains(t, response, "message")
// 				message, ok := response["message"].(string)
// 				assert.True(t, ok, "message should be a string")
// 				assert.Contains(t, message, "deleted successfully")
// 			}
// 		})
// 	}
// }

// // ==========================================
// // Get All Events Tests (Pagination)
// // ==========================================

// func TestGetAllEventsHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	// Create 5 test events
// 	for i := 1; i <= 5; i++ {
// 		createTestEvent(t, pool, user.ID, categoryID, fmt.Sprintf("Event %d", i))
// 	}

// 	tests := []struct {
// 		name           string
// 		cursor         string
// 		expectedStatus int
// 		checkPagination bool
// 	}{
// 		{
// 			name:            "First page",
// 			cursor:          "",
// 			expectedStatus:  http.StatusOK,
// 			checkPagination: true,
// 		},
// 		{
// 			name:            "With cursor",
// 			cursor:          "",  // Will be set after first request
// 			expectedStatus:  http.StatusOK,
// 			checkPagination: true,
// 		},
// 		{
// 			name:            "Invalid cursor",
// 			cursor:          "invalid-cursor",
// 			expectedStatus:  http.StatusBadRequest,
// 			checkPagination: false,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			url := "/api/events"
// 			if tt.cursor != "" {
// 				url += "?cursor=" + tt.cursor
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.GetAllEventsHandlers(pool))

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseEventsResponse(w)

// 			if tt.checkPagination {
// 				assert.Contains(t, response, "data")
// 				assert.Contains(t, response, "next_cursor")
				
// 				data := response["data"].([]interface{})
// 				assert.LessOrEqual(t, len(data), 3, "Should return max 3 events per page")
// 			}
// 		})
// 	}
// }

// // ==========================================
// // Featured Events Tests
// // ==========================================

// func TestFeaturedEventsHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	// Create featured event
// 	now := time.Now()
// 	featuredUntil := now.Add(120 * time.Hour)
// 	_, err := repository.CreateEvents(
// 		pool,
// 		"Featured Event",
// 		"A featured event",
// 		now.Add(24*time.Hour),
// 		now.Add(48*time.Hour),
// 		"https://example.com/image.jpg",
// 		0,
// 		true,
// 		now.Add(72*time.Hour),
// 		"Test Venue",
// 		"Test City",
// 		"123 Test St",
// 		categoryID,
// 		&featuredUntil,
// 		user.ID,
// 	)
// 	assert.NoError(t, err)

// 	// Create non-featured event
// 	createTestEvent(t, pool, user.ID, categoryID, "Regular Event")

// 	t.Run("Get featured events only", func(t *testing.T) {
// 		w := makeAuthenticatedRequest("GET", "/api/events/featured", nil, user.ID, handlers.FeaturedEventsHandlers(pool))

// 		assert.Equal(t, http.StatusOK, w.Code)

// 		response := parseEventsResponse(w)
// 		assert.Contains(t, response, "data")

// 		data := response["data"].([]interface{})
// 		assert.Equal(t, 1, len(data), "Should only return featured events")
// 	})
// }

// // ==========================================
// // Search Events Tests
// // ==========================================

// func TestSearchEventsHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	// Create events with different titles
// 	createTestEvent(t, pool, user.ID, categoryID, "Rock Concert")
// 	createTestEvent(t, pool, user.ID, categoryID, "Jazz Festival")
// 	createTestEvent(t, pool, user.ID, categoryID, "Classical Music Night")

// 	// Update search vectors manually since we're creating directly
// 	pool.Exec(context.Background(), `
// 		UPDATE events 
// 		SET search_vector = to_tsvector('english', title || ' ' || descript)
// 	`)

// 	tests := []struct {
// 		name            string
// 		searchQuery     string
// 		expectedStatus  int
// 		shouldHaveError bool
// 		minResults      int
// 	}{
// 		{
// 			name:            "Search for 'Concert'",
// 			searchQuery:     "Concert",
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 			minResults:      1,
// 		},
// 		{
// 			name:            "Search for 'Music'",
// 			searchQuery:     "Music",
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 			minResults:      1,
// 		},
// 		{
// 			name:            "Empty search query",
// 			searchQuery:     "",
// 			expectedStatus:  http.StatusBadRequest,
// 			shouldHaveError: true,
// 			minResults:      0,
// 		},
// 		{
// 			name:            "No results",
// 			searchQuery:     "NonExistentEvent",
// 			expectedStatus:  http.StatusOK,
// 			shouldHaveError: false,
// 			minResults:      0,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			url := "/api/events/search"
// 			if tt.searchQuery != "" {
// 				url += "?q=" + tt.searchQuery
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.SearchEventsHandler(pool))

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseEventsResponse(w)

// 			if tt.shouldHaveError {
// 				assert.Contains(t, response, "error")
// 			} else {
// 				assert.Contains(t, response, "data")
// 				if response["data"] != nil {
// 					data := response["data"].([]interface{})
// 					assert.GreaterOrEqual(t, len(data), tt.minResults)
// 				}
// 			}
// 		})
// 	}
// }

// // ==========================================
// // Trending Events Tests
// // ==========================================

// func TestTrendingEventsHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	// Create events with different view counts
// 	now := time.Now()
	
// 	// High views event
// 	repository.CreateEvents(pool, "Popular Event", "Description", now.Add(24*time.Hour), now.Add(48*time.Hour),
// 		"https://example.com/image.jpg", 100, false, now.Add(72*time.Hour),
// 		"Venue", "City", "Address", categoryID, nil, user.ID)
	
// 	// Low views event
// 	repository.CreateEvents(pool, "Regular Event", "Description", now.Add(24*time.Hour), now.Add(48*time.Hour),
// 		"https://example.com/image.jpg", 10, false, now.Add(72*time.Hour),
// 		"Venue", "City", "Address", categoryID, nil, user.ID)

// 	t.Run("Get trending events", func(t *testing.T) {
// 		w := makeAuthenticatedRequest("GET", "/api/events/trending", nil, user.ID, handlers.TrendingEventHandlers(pool))

// 		assert.Equal(t, http.StatusOK, w.Code)

// 		response := parseEventsResponse(w)
// 		assert.Contains(t, response, "data")

// 		data := response["data"].([]interface{})
// 		assert.GreaterOrEqual(t, len(data), 1, "Should return trending events")
		
// 		// First event should have more views than second
// 		if len(data) >= 2 {
// 			firstEvent := data[0].(map[string]interface{})
// 			secondEvent := data[1].(map[string]interface{})
			
// 			firstViews := int(firstEvent["views"].(float64))
// 			secondViews := int(secondEvent["views"].(float64))
			
// 			assert.GreaterOrEqual(t, firstViews, secondViews, "Events should be sorted by views DESC")
// 		}
// 	})
// }

// // ==========================================
// // Active Events Tests
// // ==========================================

// func TestActiveEventsHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	now := time.Now()

// 	// Create future event (active)
// 	repository.CreateEvents(pool, "Future Event", "Description", now.Add(24*time.Hour), now.Add(48*time.Hour),
// 		"https://example.com/image.jpg", 0, false, now.Add(72*time.Hour),
// 		"Venue", "City", "Address", categoryID, nil, user.ID)

// 	// Create past event (not active)
// 	repository.CreateEvents(pool, "Past Event", "Description", now.Add(-48*time.Hour), now.Add(-24*time.Hour),
// 		"https://example.com/image.jpg", 0, false, now.Add(72*time.Hour),
// 		"Venue", "City", "Address", categoryID, nil, user.ID)

// 	t.Run("Get only active events", func(t *testing.T) {
// 		w := makeAuthenticatedRequest("GET", "/api/events/active", nil, user.ID, handlers.ActiveEventsHandlers(pool))

// 		assert.Equal(t, http.StatusOK, w.Code)

// 		response := parseEventsResponse(w)
// 		assert.Contains(t, response, "data")

// 		data := response["data"].([]interface{})
		
// 		// Should only return events that haven't ended yet
// 		for _, item := range data {
// 			event := item.(map[string]interface{})
// 			assert.Contains(t, event, "end_time")
// 		}
// 	})
// }

// // ==========================================
// // Get Events by Category Tests
// // ==========================================

// func TestGetEventsByCategoryHandler(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	musicCategoryID := createTestCategory(t, pool, "Music")
// 	sportsCategoryID := createTestCategory(t, pool, "Sports")

// 	// Create music events
// 	createTestEvent(t, pool, user.ID, musicCategoryID, "Music Event 1")
// 	createTestEvent(t, pool, user.ID, musicCategoryID, "Music Event 2")

// 	// Create sports event
// 	createTestEvent(t, pool, user.ID, sportsCategoryID, "Sports Event 1")

// 	tests := []struct {
// 		name           string
// 		categoryID     int
// 		expectedStatus int
// 		minResults     int
// 	}{
// 		{
// 			name:           "Get music events",
// 			categoryID:     musicCategoryID,
// 			expectedStatus: http.StatusOK,
// 			minResults:     2,
// 		},
// 		{
// 			name:           "Get sports events",
// 			categoryID:     sportsCategoryID,
// 			expectedStatus: http.StatusOK,
// 			minResults:     1,
// 		},
// 		{
// 			name:           "Invalid category ID",
// 			categoryID:     999,
// 			expectedStatus: http.StatusOK,
// 			minResults:     0,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			categoryIDStr := fmt.Sprintf("%d", tt.categoryID)
// 			url := "/api/events/category/" + categoryIDStr

// 			w := makeAuthenticatedRequestWithParams("GET", url, nil, user.ID,
// 				handlers.GetEventCategory(pool), gin.Params{{Key: "id", Value: categoryIDStr}})

// 			assert.Equal(t, tt.expectedStatus, w.Code)

// 			response := parseEventsResponse(w)
// 			assert.Contains(t, response, "data")

// 			data := response["data"].([]interface{})
// 			assert.GreaterOrEqual(t, len(data), tt.minResults)
// 		})
// 	}
// }

// // ==========================================
// // Cursor Pagination Tests (Comprehensive)
// // ==========================================

// func TestCursorPagination(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	// Create 10 events (more than page size of 3)
// 	now := time.Now()
// 	createdEvents := make([]*models.Events, 10)
// 	for i := 0; i < 10; i++ {
// 		event, err := repository.CreateEvents(
// 			pool,
// 			fmt.Sprintf("Event %d", i+1),
// 			"Test event description",
// 			now.Add(time.Duration(i)*time.Hour),  // Different start times for sorting
// 			now.Add(time.Duration(i+24)*time.Hour),
// 			"https://example.com/image.jpg",
// 			i*10, // Different view counts for trending
// 			false,
// 			now.Add(72*time.Hour),
// 			"Test Venue",
// 			"Test City",
// 			"123 Test St",
// 			categoryID,
// 			nil,
// 			user.ID,
// 		)
// 		assert.NoError(t, err, "Failed to create test event")
// 		createdEvents[i] = event
// 	}

// 	t.Run("GetAllEvents - Full pagination flow", func(t *testing.T) {
// 		var allFetchedEvents []interface{}
// 		var cursor string
// 		pageCount := 0

// 		// Fetch all pages
// 		for {
// 			pageCount++
			
// 			url := "/api/events"
// 			if cursor != "" {
// 				url += "?cursor=" + cursor
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.GetAllEventsHandlers(pool))

// 			assert.Equal(t, http.StatusOK, w.Code, "Page %d should return 200", pageCount)

// 			response := parseEventsResponse(w)
// 			assert.Contains(t, response, "data", "Page %d should have data", pageCount)
// 			assert.Contains(t, response, "next_cursor", "Page %d should have next_cursor", pageCount)

// 			data := response["data"].([]interface{})
			
// 			// Each page should have max 3 events (except possibly the last)
// 			assert.LessOrEqual(t, len(data), 3, "Page %d should have max 3 events", pageCount)
			
// 			// Add to all fetched events
// 			allFetchedEvents = append(allFetchedEvents, data...)

// 			// Get next cursor
// 			nextCursor := response["next_cursor"]
// 			if nextCursor == nil || nextCursor == "" {
// 				// No more pages
// 				break
// 			}

// 			cursor = nextCursor.(string)
			
// 			// Safety check - don't infinite loop
// 			if pageCount > 10 {
// 				t.Fatal("Too many pages - possible infinite loop")
// 			}
// 		}

// 		// Verify we fetched all 10 events
// 		assert.Equal(t, 10, len(allFetchedEvents), "Should fetch all 10 events across pages")
		
// 		// Verify we needed multiple pages
// 		assert.GreaterOrEqual(t, pageCount, 4, "Should need at least 4 pages for 10 events with page size 3")
// 	})

// 	t.Run("GetAllEvents - Events ordered correctly", func(t *testing.T) {
// 		// First page
// 		w := makeAuthenticatedRequest("GET", "/api/events", nil, user.ID, handlers.GetAllEventsHandlers(pool))
		
// 		response := parseEventsResponse(w)
// 		data := response["data"].([]interface{})

// 		// Events should be ordered by start_time DESC
// 		for i := 0; i < len(data)-1; i++ {
// 			currentEvent := data[i].(map[string]interface{})
// 			nextEvent := data[i+1].(map[string]interface{})
			
// 			currentTime, _ := time.Parse(time.RFC3339, currentEvent["start_time"].(string))
// 			nextTime, _ := time.Parse(time.RFC3339, nextEvent["start_time"].(string))
			
// 			assert.True(t, currentTime.After(nextTime) || currentTime.Equal(nextTime), 
// 				"Events should be ordered by start_time DESC")
// 		}
// 	})

// 	t.Run("GetAllEvents - No duplicate events across pages", func(t *testing.T) {
// 		seenIDs := make(map[string]bool)
// 		cursor := ""

// 		// Fetch all pages and check for duplicates
// 		for {
// 			url := "/api/events"
// 			if cursor != "" {
// 				url += "?cursor=" + cursor
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.GetAllEventsHandlers(pool))
// 			response := parseEventsResponse(w)
// 			data := response["data"].([]interface{})

// 			// Check each event ID
// 			for _, item := range data {
// 				event := item.(map[string]interface{})
// 				eventID := event["id"].(string)
				
// 				assert.False(t, seenIDs[eventID], "Event %s appears multiple times!", eventID)
// 				seenIDs[eventID] = true
// 			}

// 			// Get next cursor
// 			nextCursor := response["next_cursor"]
// 			if nextCursor == nil || nextCursor == "" {
// 				break
// 			}
// 			cursor = nextCursor.(string)
// 		}

// 		// Should have seen all 10 unique events
// 		assert.Equal(t, 10, len(seenIDs), "Should have 10 unique events")
// 	})

// 	t.Run("TrendingEvents - Pagination and ordering by views", func(t *testing.T) {
// 		// First page
// 		w := makeAuthenticatedRequest("GET", "/api/events/trending", nil, user.ID, handlers.TrendingEventHandlers(pool))
		
// 		assert.Equal(t, http.StatusOK, w.Code)
		
// 		response := parseEventsResponse(w)
// 		data := response["data"].([]interface{})

// 		// Should have events
// 		assert.Greater(t, len(data), 0, "Should have trending events")

// 		// Events should be ordered by views DESC
// 		for i := 0; i < len(data)-1; i++ {
// 			currentEvent := data[i].(map[string]interface{})
// 			nextEvent := data[i+1].(map[string]interface{})
			
// 			currentViews := int(currentEvent["views"].(float64))
// 			nextViews := int(nextEvent["views"].(float64))
			
// 			assert.GreaterOrEqual(t, currentViews, nextViews, 
// 				"Trending events should be ordered by views DESC")
// 		}

// 		// Test cursor works
// 		nextCursor := response["next_cursor"]
// 		if nextCursor != nil && nextCursor != "" {
// 			w2 := makeAuthenticatedRequest("GET", "/api/events/trending?cursor="+nextCursor.(string), 
// 				nil, user.ID, handlers.TrendingEventHandlers(pool))
			
// 			assert.Equal(t, http.StatusOK, w2.Code)
			
// 			response2 := parseEventsResponse(w2)
// 			data2 := response2["data"].([]interface{})
			
// 			// Second page should also have events
// 			assert.Greater(t, len(data2), 0, "Second page should have events")
			
// 			// First event of page 2 should have less/equal views than last event of page 1
// 			lastEventPage1 := data[len(data)-1].(map[string]interface{})
// 			firstEventPage2 := data2[0].(map[string]interface{})
			
// 			lastViews := int(lastEventPage1["views"].(float64))
// 			firstViews := int(firstEventPage2["views"].(float64))
			
// 			assert.LessOrEqual(t, firstViews, lastViews, 
// 				"Page 2 first event should have less/equal views than page 1 last event")
// 		}
// 	})

// 	t.Run("ActiveEvents - Pagination", func(t *testing.T) {
// 		var allActiveEvents []interface{}
// 		cursor := ""

// 		// Fetch all pages of active events
// 		for {
// 			url := "/api/events/active"
// 			if cursor != "" {
// 				url += "?cursor=" + cursor
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.ActiveEventsHandlers(pool))
// 			assert.Equal(t, http.StatusOK, w.Code)

// 			response := parseEventsResponse(w)
// 			data := response["data"].([]interface{})
			
// 			allActiveEvents = append(allActiveEvents, data...)

// 			nextCursor := response["next_cursor"]
// 			if nextCursor == nil || nextCursor == "" {
// 				break
// 			}
// 			cursor = nextCursor.(string)
// 		}

// 		// All events we created should be active (end_time in future)
// 		assert.Equal(t, 10, len(allActiveEvents), "All 10 events should be active")
// 	})

// 	t.Run("GetEventsByCategory - Pagination", func(t *testing.T) {
// 		categoryIDStr := fmt.Sprintf("%d", categoryID)
		
// 		var allCategoryEvents []interface{}
// 		cursor := ""

// 		// Fetch all pages
// 		for {
// 			url := "/api/events/category/" + categoryIDStr
// 			if cursor != "" {
// 				url += "?cursor=" + cursor
// 			}

// 			w := makeAuthenticatedRequestWithParams("GET", url, nil, user.ID,
// 				handlers.GetEventCategory(pool), gin.Params{{Key: "id", Value: categoryIDStr}})
			
// 			assert.Equal(t, http.StatusOK, w.Code)

// 			response := parseEventsResponse(w)
// 			data := response["data"].([]interface{})
			
// 			allCategoryEvents = append(allCategoryEvents, data...)

// 			nextCursor := response["next_cursor"]
// 			if nextCursor == nil || nextCursor == "" {
// 				break
// 			}
// 			cursor = nextCursor.(string)
// 		}

// 		// All 10 events are in Music category
// 		assert.Equal(t, 10, len(allCategoryEvents), "Should get all 10 Music events")
// 	})

// 	t.Run("Invalid cursor - should fail gracefully", func(t *testing.T) {
// 		w := makeAuthenticatedRequest("GET", "/api/events?cursor=invalid-cursor-123", 
// 			nil, user.ID, handlers.GetAllEventsHandlers(pool))
		
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
		
// 		response := parseEventsResponse(w)
// 		assert.Contains(t, response, "error")
// 		assert.Contains(t, response["error"], "Invalid cursor")
// 	})

// 	t.Run("Empty cursor - should return first page", func(t *testing.T) {
// 		w1 := makeAuthenticatedRequest("GET", "/api/events", nil, user.ID, handlers.GetAllEventsHandlers(pool))
// 		w2 := makeAuthenticatedRequest("GET", "/api/events?cursor=", nil, user.ID, handlers.GetAllEventsHandlers(pool))
		
// 		response1 := parseEventsResponse(w1)
// 		response2 := parseEventsResponse(w2)
		
// 		// Both should return the same first page
// 		data1 := response1["data"].([]interface{})
// 		data2 := response2["data"].([]interface{})
		
// 		assert.Equal(t, len(data1), len(data2), "Empty cursor should return first page")
		
// 		// First events should be the same
// 		if len(data1) > 0 && len(data2) > 0 {
// 			event1 := data1[0].(map[string]interface{})
// 			event2 := data2[0].(map[string]interface{})
// 			assert.Equal(t, event1["id"], event2["id"], "First events should match")
// 		}
// 	})

// 	t.Run("Page size consistency", func(t *testing.T) {
// 		// First page should have exactly 3 events (since we have 10 total)
// 		w := makeAuthenticatedRequest("GET", "/api/events", nil, user.ID, handlers.GetAllEventsHandlers(pool))
		
// 		response := parseEventsResponse(w)
// 		data := response["data"].([]interface{})
		
// 		assert.Equal(t, 3, len(data), "First page should have exactly 3 events")
		
// 		// Should have next_cursor
// 		assert.NotNil(t, response["next_cursor"], "Should have next_cursor when more events exist")
// 		assert.NotEqual(t, "", response["next_cursor"], "next_cursor should not be empty")
// 	})

// 	t.Run("Last page - no next_cursor", func(t *testing.T) {
// 		cursor := ""
// 		var lastResponse map[string]interface{}

// 		// Navigate to last page
// 		for {
// 			url := "/api/events"
// 			if cursor != "" {
// 				url += "?cursor=" + cursor
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.GetAllEventsHandlers(pool))
// 			response := parseEventsResponse(w)
			
// 			lastResponse = response

// 			nextCursor := response["next_cursor"]
// 			if nextCursor == nil || nextCursor == "" {
// 				break
// 			}
// 			cursor = nextCursor.(string)
// 		}

// 		// Last page should have no next_cursor or empty next_cursor
// 		nextCursor := lastResponse["next_cursor"]
// 		assert.True(t, nextCursor == nil || nextCursor == "", 
// 			"Last page should have no next_cursor")
		
// 		// Last page might have less than 3 events
// 		data := lastResponse["data"].([]interface{})
// 		assert.LessOrEqual(t, len(data), 3, "Last page should have max 3 events")
// 		assert.Greater(t, len(data), 0, "Last page should have at least 1 event")
// 	})
// }

// // ==========================================
// // Featured Events Pagination Test
// // ==========================================

// func TestFeaturedEventsPagination(t *testing.T) {
// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	// Create 8 featured events with different featured_until times
// 	now := time.Now()
// 	for i := 0; i < 8; i++ {
// 		featuredUntil := now.Add(time.Duration(100+i) * time.Hour)
// 		_, err := repository.CreateEvents(
// 			pool,
// 			fmt.Sprintf("Featured Event %d", i+1),
// 			"A featured event",
// 			now.Add(24*time.Hour),
// 			now.Add(48*time.Hour),
// 			"https://example.com/image.jpg",
// 			0,
// 			true,
// 			now.Add(72*time.Hour),
// 			"Test Venue",
// 			"Test City",
// 			"123 Test St",
// 			categoryID,
// 			&featuredUntil,
// 			user.ID,
// 		)
// 		assert.NoError(t, err)
// 	}

// 	// Create 2 non-featured events
// 	createTestEvent(t, pool, user.ID, categoryID, "Regular Event 1")
// 	createTestEvent(t, pool, user.ID, categoryID, "Regular Event 2")

// 	t.Run("Featured events pagination - only returns featured", func(t *testing.T) {
// 		var allFeaturedEvents []interface{}
// 		cursor := ""

// 		// Fetch all pages
// 		for {
// 			url := "/api/events/featured"
// 			if cursor != "" {
// 				url += "?cursor=" + cursor
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.FeaturedEventsHandlers(pool))
// 			assert.Equal(t, http.StatusOK, w.Code)

// 			response := parseEventsResponse(w)
// 			data := response["data"].([]interface{})
			
// 			// Verify all events are featured
// 			for _, item := range data {
// 				event := item.(map[string]interface{})
// 				assert.Equal(t, true, event["is_featured"], "All events should be featured")
// 				assert.NotNil(t, event["featured_until"], "Featured events should have featured_until")
// 			}
			
// 			allFeaturedEvents = append(allFeaturedEvents, data...)

// 			nextCursor := response["next_cursor"]
// 			if nextCursor == nil || nextCursor == "" {
// 				break
// 			}
// 			cursor = nextCursor.(string)
// 		}

// 		// Should only get 8 featured events, not the 2 regular ones
// 		assert.Equal(t, 8, len(allFeaturedEvents), "Should get only 8 featured events")
// 	})

// 	t.Run("Featured events - ordered by featured_until DESC", func(t *testing.T) {
// 		w := makeAuthenticatedRequest("GET", "/api/events/featured", nil, user.ID, handlers.FeaturedEventsHandlers(pool))
		
// 		response := parseEventsResponse(w)
// 		data := response["data"].([]interface{})

// 		// Check ordering
// 		for i := 0; i < len(data)-1; i++ {
// 			currentEvent := data[i].(map[string]interface{})
// 			nextEvent := data[i+1].(map[string]interface{})
			
// 			currentFeatured, _ := time.Parse(time.RFC3339, currentEvent["featured_until"].(string))
// 			nextFeatured, _ := time.Parse(time.RFC3339, nextEvent["featured_until"].(string))
			
// 			assert.True(t, currentFeatured.After(nextFeatured) || currentFeatured.Equal(nextFeatured),
// 				"Featured events should be ordered by featured_until DESC")
// 		}
// 	})
// }

// // ==========================================
// // Performance Test for Pagination
// // ==========================================

// func TestPaginationPerformance(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip("Skipping performance test in short mode")
// 	}

// 	pool := setupEventsTestDB(t)
// 	defer cleanupEventsTestDB(pool)

// 	user := createEventsTestUser(t, pool, "test@example.com", "password123", "user")
// 	categoryID := createTestCategory(t, pool, "Music")

// 	// Create 100 events
// 	now := time.Now()
// 	t.Log("Creating 100 test events...")
// 	for i := 0; i < 100; i++ {
// 		repository.CreateEvents(
// 			pool,
// 			fmt.Sprintf("Event %d", i+1),
// 			"Test event",
// 			now.Add(time.Duration(i)*time.Hour),
// 			now.Add(time.Duration(i+24)*time.Hour),
// 			"https://example.com/image.jpg",
// 			i,
// 			false,
// 			now.Add(72*time.Hour),
// 			"Test Venue",
// 			"Test City",
// 			"123 Test St",
// 			categoryID,
// 			nil,
// 			user.ID,
// 		)
// 	}

// 	t.Run("Fetch all 100 events efficiently", func(t *testing.T) {
// 		start := time.Now()
		
// 		var allEvents []interface{}
// 		cursor := ""
// 		pageCount := 0

// 		for {
// 			pageCount++
// 			url := "/api/events"
// 			if cursor != "" {
// 				url += "?cursor=" + cursor
// 			}

// 			w := makeAuthenticatedRequest("GET", url, nil, user.ID, handlers.GetAllEventsHandlers(pool))
// 			response := parseEventsResponse(w)
// 			data := response["data"].([]interface{})
			
// 			allEvents = append(allEvents, data...)

// 			nextCursor := response["next_cursor"]
// 			if nextCursor == nil || nextCursor == "" {
// 				break
// 			}
// 			cursor = nextCursor.(string)
// 		}

// 		elapsed := time.Since(start)

// 		assert.Equal(t, 100, len(allEvents), "Should fetch all 100 events")
		
// 		// Should take less than 2 seconds to fetch 100 events
// 		assert.Less(t, elapsed.Seconds(), 2.0, 
// 			"Fetching 100 events should take less than 2 seconds (took %v)", elapsed)
		
// 		t.Logf("Fetched 100 events in %d pages in %v", pageCount, elapsed)
// 	})
// }