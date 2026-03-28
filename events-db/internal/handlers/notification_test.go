package handlers_test

// =============================================================================
// NOTIFICATION TESTS
// =============================================================================
//
// Tests covered:
//   GET  /notifications         → GetNotificationsHandler
//     - returns empty list when user has no notifications
//     - returns notifications in descending created_at order
//     - only returns notifications for the requesting user
//     - unauthenticated request gets 401
//
//   GET  /notifications/unread  → GetUnreadCountHandler
//     - returns 0 when no notifications exist
//     - returns correct unread count
//     - count decreases after marking as read
//     - unauthenticated request gets 401
//
//   PATCH /notifications/:id/read → MarkReadHandler
//     - marks a notification as read successfully
//     - cannot mark another user's notification as read — 404
//     - invalid ID returns 404
//     - unauthenticated request gets 401
//
//   POST /notifications/read-all → MarkAllReadHandler
//     - marks all notifications as read
//     - no-op when already all read — still 200
//     - unauthenticated request gets 401
//
// Run:
//   go test ./internal/handlers/... -v -run TestNotif -count=1
// =============================================================================

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"events-db/internal/config"
	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ─── DB setup ─────────────────────────────────────────────────────────────────

func setupNotifTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("notif DB tests skipped — DB not reachable: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("notif DB tests skipped — DB not reachable: %v", pingErr)
	}
	createNotifTestTables(t, pool)
	cleanNotifTables(t, pool)
	return pool
}

func createNotifTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			password   TEXT NOT NULL,
			role       VARCHAR(50) NOT NULL DEFAULT 'user',
			first_name TEXT,
			last_name  TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT UNIQUE NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked    BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS notifications (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			type       TEXT NOT NULL,
			title      TEXT NOT NULL,
			body       TEXT NOT NULL,
			data       JSONB,
			is_read    BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Logf("table setup warning: %v", err)
		}
	}
}

func cleanNotifTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE notifications CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func buildNotifRouter(pool *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	notifs := r.Group("/notifications")
	notifs.Use(middleware.AuthMiddleware(cfg))
	{
		notifs.GET("",            handlers.GetNotificationsHandler(pool))
		notifs.GET("/unread",     handlers.GetUnreadCountHandler(pool))
		notifs.PATCH("/:id/read", handlers.MarkReadHandler(pool))
		notifs.POST("/read-all",  handlers.MarkAllReadHandler(pool))
	}
	return r
}

// createNotifUser creates a fresh user and returns their ID and access token.
func createNotifUser(t *testing.T, pool *pgxpool.Pool, cfg *config.Config) (userID, token string) {
	t.Helper()
	email := randomEmail()
	password := "Password1234!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	u := &models.User{Email: email, Password: string(hashed), Role: "user"}
	created, err := repository.CreateUser(pool, u)
	require.NoError(t, err)
	userID = created.ID

	r := buildAuthRouter(pool, cfg)
	w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": password,
	})
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token = resp["access_token"].(string)
	return
}

// seedNotification inserts a notification directly via the repository.
func seedNotification(t *testing.T, pool *pgxpool.Pool, userID string, notifType models.NotificationType, title, body string) {
	t.Helper()
	err := repository.CreateNotification(
		pool, context.Background(),
		userID, notifType, title, body,
		map[string]string{"event_id": "evt-test-123"},
	)
	require.NoError(t, err)
}

// ─── GET /notifications ───────────────────────────────────────────────────────

func TestNotifGetList(t *testing.T) {
	pool := setupNotifTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()
	r := buildNotifRouter(pool, cfg)

	t.Run("returns empty list when user has no notifications", func(t *testing.T) {
		cleanNotifTables(t, pool)
		_, token := createNotifUser(t, pool, cfg)

		w := doAuthRequest(r, http.MethodGet, "/notifications", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 0)
	})

	t.Run("returns notifications in descending created_at order", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userID, token := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "First",  "first body")
		seedNotification(t, pool, userID, models.NotifEventCancelled,   "Second", "second body")
		seedNotification(t, pool, userID, models.NotifRefundProcessed,  "Third",  "third body")

		w := doAuthRequest(r, http.MethodGet, "/notifications", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 3)

		// Most recent first
		assert.Equal(t, "Third",  data[0].(map[string]interface{})["title"])
		assert.Equal(t, "Second", data[1].(map[string]interface{})["title"])
		assert.Equal(t, "First",  data[2].(map[string]interface{})["title"])
	})

	t.Run("only returns notifications for the requesting user", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userAID, tokenA := createNotifUser(t, pool, cfg)
		userBID, _      := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userAID, models.NotifBookingConfirmed, "User A notif", "for A")
		seedNotification(t, pool, userBID, models.NotifBookingConfirmed, "User B notif", "for B")

		w := doAuthRequest(r, http.MethodGet, "/notifications", nil, tokenA)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)
		assert.Equal(t, "User A notif", data[0].(map[string]interface{})["title"])
	})

	t.Run("unauthenticated request gets 401", func(t *testing.T) {
		w := doRequest(r, http.MethodGet, "/notifications", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GET /notifications/unread ────────────────────────────────────────────────

func TestNotifUnreadCount(t *testing.T) {
	pool := setupNotifTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()
	r := buildNotifRouter(pool, cfg)

	t.Run("returns 0 when no notifications exist", func(t *testing.T) {
		cleanNotifTables(t, pool)
		_, token := createNotifUser(t, pool, cfg)

		w := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 0, resp["unread_count"])
	})

	t.Run("returns correct unread count", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userID, token := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Notif 1", "body 1")
		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Notif 2", "body 2")
		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Notif 3", "body 3")

		w := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 3, resp["unread_count"])
	})

	t.Run("count decreases after marking one as read", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userID, token := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Notif 1", "body 1")
		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Notif 2", "body 2")

		// Fetch the list to get a real notification ID
		listW := doAuthRequest(r, http.MethodGet, "/notifications", nil, token)
		var listResp map[string]interface{}
		require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listResp))
		notifID := listResp["data"].([]interface{})[0].(map[string]interface{})["id"].(string)

		// Mark one as read
		doAuthRequest(r, http.MethodPatch, "/notifications/"+notifID+"/read", nil, token)

		// Unread count should now be 1
		w := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, token)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 1, resp["unread_count"])
	})

	t.Run("unauthenticated request gets 401", func(t *testing.T) {
		w := doRequest(r, http.MethodGet, "/notifications/unread", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── PATCH /notifications/:id/read ───────────────────────────────────────────

func TestNotifMarkRead(t *testing.T) {
	pool := setupNotifTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()
	r := buildNotifRouter(pool, cfg)

	t.Run("marks a notification as read successfully", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userID, token := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Test Notif", "body")

		// Get the ID
		listW := doAuthRequest(r, http.MethodGet, "/notifications", nil, token)
		var listResp map[string]interface{}
		require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listResp))
		notif := listResp["data"].([]interface{})[0].(map[string]interface{})
		notifID := notif["id"].(string)
		assert.Equal(t, false, notif["is_read"])

		// Mark as read
		w := doAuthRequest(r, http.MethodPatch, "/notifications/"+notifID+"/read", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		// Confirm is_read is now true
		listW2 := doAuthRequest(r, http.MethodGet, "/notifications", nil, token)
		var listResp2 map[string]interface{}
		require.NoError(t, json.Unmarshal(listW2.Body.Bytes(), &listResp2))
		notif2 := listResp2["data"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, true, notif2["is_read"])
	})

	t.Run("cannot mark another user's notification as read — 404", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userAID, tokenA := createNotifUser(t, pool, cfg)
		_,       tokenB := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userAID, models.NotifBookingConfirmed, "User A notif", "body")

		// Get user A's notification ID
		listW := doAuthRequest(r, http.MethodGet, "/notifications", nil, tokenA)
		_ = listW // user B has no notifications

		// Get the ID directly via repository
		notifs, _, err := repository.GetNotifications(pool, userAID, 10, nil)
		require.NoError(t, err)
		require.Len(t, notifs, 1)

		// User B tries to mark user A's notification
		w := doAuthRequest(r, http.MethodPatch, "/notifications/"+notifs[0].ID+"/read", nil, tokenB)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("non-existent notification ID returns 404", func(t *testing.T) {
		cleanNotifTables(t, pool)
		_, token := createNotifUser(t, pool, cfg)

		w := doAuthRequest(r, http.MethodPatch, "/notifications/00000000-0000-0000-0000-000000000000/read", nil, token)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unauthenticated request gets 401", func(t *testing.T) {
		w := doRequest(r, http.MethodPatch, "/notifications/some-id/read", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── POST /notifications/read-all ────────────────────────────────────────────

func TestNotifMarkAllRead(t *testing.T) {
	pool := setupNotifTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()
	r := buildNotifRouter(pool, cfg)

	t.Run("marks all notifications as read", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userID, token := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Notif 1", "body 1")
		seedNotification(t, pool, userID, models.NotifEventCancelled,   "Notif 2", "body 2")
		seedNotification(t, pool, userID, models.NotifRefundProcessed,  "Notif 3", "body 3")

		// Confirm all unread
		unreadW := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, token)
		var unreadResp map[string]interface{}
		require.NoError(t, json.Unmarshal(unreadW.Body.Bytes(), &unreadResp))
		assert.EqualValues(t, 3, unreadResp["unread_count"])

		// Mark all read
		w := doAuthRequest(r, http.MethodPost, "/notifications/read-all", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		// Unread count should now be 0
		unreadW2 := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, token)
		var unreadResp2 map[string]interface{}
		require.NoError(t, json.Unmarshal(unreadW2.Body.Bytes(), &unreadResp2))
		assert.EqualValues(t, 0, unreadResp2["unread_count"])
	})

	t.Run("no-op when all already read — still returns 200", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userID, token := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userID, models.NotifBookingConfirmed, "Notif 1", "body 1")

		// Mark all read once
		doAuthRequest(r, http.MethodPost, "/notifications/read-all", nil, token)

		// Mark all read again — should still be 200
		w := doAuthRequest(r, http.MethodPost, "/notifications/read-all", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		// Count still 0
		unreadW := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, token)
		var unreadResp map[string]interface{}
		require.NoError(t, json.Unmarshal(unreadW.Body.Bytes(), &unreadResp))
		assert.EqualValues(t, 0, unreadResp["unread_count"])
	})

	t.Run("only marks the requesting user's notifications as read", func(t *testing.T) {
		cleanNotifTables(t, pool)
		userAID, tokenA := createNotifUser(t, pool, cfg)
		userBID, tokenB := createNotifUser(t, pool, cfg)

		seedNotification(t, pool, userAID, models.NotifBookingConfirmed, "User A notif", "body")
		seedNotification(t, pool, userBID, models.NotifBookingConfirmed, "User B notif", "body")

		// User A marks all as read
		doAuthRequest(r, http.MethodPost, "/notifications/read-all", nil, tokenA)

		// User A's count should be 0
		wA := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, tokenA)
		var respA map[string]interface{}
		require.NoError(t, json.Unmarshal(wA.Body.Bytes(), &respA))
		assert.EqualValues(t, 0, respA["unread_count"])

		// User B's count should still be 1
		wB := doAuthRequest(r, http.MethodGet, "/notifications/unread", nil, tokenB)
		var respB map[string]interface{}
		require.NoError(t, json.Unmarshal(wB.Body.Bytes(), &respB))
		assert.EqualValues(t, 1, respB["unread_count"])
	})

	t.Run("unauthenticated request gets 401", func(t *testing.T) {
		w := doRequest(r, http.MethodPost, "/notifications/read-all", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}