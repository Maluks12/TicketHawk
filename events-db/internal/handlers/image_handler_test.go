 package handlers_test

// // =============================================================================
// // IMAGE HANDLER TESTS  (self-contained — no manual Docker setup required)
// // =============================================================================
// //
// // These tests spin up a real Postgres container automatically via
// // testcontainers-go.  Docker must be running on the host, but you do NOT need
// // to start or manage any container yourself.
// //
// // Tests covered:
// //
// //   UploadCoverImageHandler
// //     • happy path → 200, URL in response body
// //     • missing "image" form field → 400
// //     • non-UUID event ID in path → 400
// //     • event not owned by the calling organizer → 404
// //     • unsupported MIME type (GIF) → 400
// //     • file exceeds 5 MB cover cap → 400
// //     • replaces existing cover: old R2 object deleted
// //     • unauthenticated request → 401
// //
// //   UploadBannerImageHandler
// //     • happy path → 200, URL in response body
// //     • file exceeds 8 MB banner cap → 400
// //     • replaces existing banner: old R2 object deleted
// //
// //   UploadGalleryImageHandler
// //     • happy path → 200, URL in response body
// //     • gallery URL actually persisted in gallery_images JSONB column
// //     • event not owned by caller → 404, no R2 upload
// //     • non-existent event UUID → 404, no R2 upload
// //
// //   Repository layer (direct calls, no HTTP)
// //     • UpdateEventCoverImage happy path → column updated in DB
// //     • UpdateEventCoverImage wrong owner → ErrEventNotFound
// //     • AppendGalleryImage accumulates multiple URLs
// //     • GetEventImageURLs returns nil pointers when columns are NULL
// //
// // Run:
// //   go test ./internal/handlers/... -v -run TestImage
// //   go test ./internal/handlers/... -v -run TestImageRepo
// // =============================================================================

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"mime/multipart"
// 	"net/http"
// 	"net/http/httptest"
// 	"net/textproto"
// 	"sync"
// 	"testing"
// 	"time"

// 	"events-db/internal/config"
// 	"events-db/internal/handlers"
// 	"events-db/internal/middleware"
// 	"events-db/internal/repository"
// 	"events-db/internal/services"

// 	"github.com/gin-gonic/gin"
// 	"github.com/golang-jwt/jwt/v5"
// 	"github.com/google/uuid"
// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// 	"github.com/testcontainers/testcontainers-go"
// 	"github.com/testcontainers/testcontainers-go/modules/postgres"
// 	"github.com/testcontainers/testcontainers-go/wait"
// 	"golang.org/x/crypto/bcrypt"
// )

// // ─── minimal test JPEG (1×1 pixel, 22 bytes) ─────────────────────────────────

// var handlerJPEG = []byte{
// 	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
// 	0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
// 	0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9,
// }

// // ─── testcontainers Postgres fixture ─────────────────────────────────────────

// // startPostgres spins up a throwaway Postgres 15 container, applies the
// // minimal schema required by the image tests, and returns a ready pool.
// // The container is automatically terminated when t finishes.
// func startPostgres(t *testing.T) *pgxpool.Pool {
// 	t.Helper()
// 	ctx := context.Background()

// 	ctr, err := postgres.Run(ctx,
// 		"postgres:15-alpine",
// 		postgres.WithDatabase("test_db"),
// 		postgres.WithUsername("testuser"),
// 		postgres.WithPassword("testpass"),
// 		testcontainers.WithWaitStrategy(
// 			wait.ForLog("database system is ready to accept connections").
// 				WithOccurrence(2).
// 				WithStartupTimeout(60*time.Second),
// 		),
// 	)
// 	require.NoError(t, err, "failed to start Postgres container — is Docker running?")
// 	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

// 	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
// 	require.NoError(t, err)

// 	pool, err := pgxpool.New(ctx, dsn)
// 	require.NoError(t, err)
// 	t.Cleanup(pool.Close)

// 	applyImgSchema(t, pool)
// 	return pool
// }

// func applyImgSchema(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	_, err := pool.Exec(context.Background(), `
// 		CREATE EXTENSION IF NOT EXISTS "pgcrypto";

// 		CREATE TABLE IF NOT EXISTS users (
// 			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			email         TEXT UNIQUE NOT NULL,
// 			password_hash TEXT NOT NULL,
// 			role          TEXT NOT NULL DEFAULT 'attendee',
// 			is_verified   BOOLEAN NOT NULL DEFAULT false,
// 			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
// 			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
// 		);

// 		CREATE TABLE IF NOT EXISTS events (
// 			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			organizer_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
// 			title            TEXT NOT NULL,
// 			cover_image_url  TEXT,
// 			banner_image_url TEXT,
// 			gallery_images   JSONB,
// 			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
// 			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
// 		);
// 	`)
// 	require.NoError(t, err)
// }

// // ─── tracked R2 mock ──────────────────────────────────────────────────────────

// type r2Mock struct {
// 	mu      sync.Mutex
// 	puts    int
// 	deletes int
// 	srv     *httptest.Server
// }

// func newR2Mock(t *testing.T) *r2Mock {
// 	t.Helper()
// 	m := &r2Mock{}
// 	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		_, _ = io.Copy(io.Discard, r.Body)
// 		m.mu.Lock()
// 		defer m.mu.Unlock()
// 		switch r.Method {
// 		case http.MethodPut:
// 			m.puts++
// 			w.WriteHeader(http.StatusOK)
// 		case http.MethodDelete:
// 			m.deletes++
// 			w.WriteHeader(http.StatusNoContent)
// 		default:
// 			w.WriteHeader(http.StatusMethodNotAllowed)
// 		}
// 	}))
// 	t.Cleanup(m.srv.Close)
// 	return m
// }

// func (m *r2Mock) URL() string {
// 	return m.srv.URL
// }
// func (m *r2Mock) Puts() int {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	return m.puts
// }
// func (m *r2Mock) Deletes() int {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	return m.deletes
// }

// // ─── config / router factories ────────────────────────────────────────────────

// func imgHandlerCfg(r2URL string) *config.Config {
// 	return &config.Config{
// 		JWTSecret:           "test-secret-that-is-at-least-32-chars!!",
// 		FrontendURL:         "http://localhost:3000",
// 		SendGridAPIKey:      "SG.test",
// 		StitchClientID:      "test-client",
// 		StitchClientSecret:  "test-secret",
// 		StitchWebhookSecret: "whsec_test",
// 		EmailFromAddress:    "no-reply@test.com",
// 		EmailFromName:       "Test",
// 		R2AccessKey:         "test-access",
// 		R2SecretKey:         "test-secret-key",
// 		R2Bucket:            "test-bucket",
// 		R2URL:               r2URL,
// 	}
// }

// func imgRouter(pool *pgxpool.Pool, svc *services.ImageService, cfg *config.Config) *gin.Engine {
// 	gin.SetMode(gin.TestMode)
// 	r := gin.New()
// 	grp := r.Group("/events/:id", middleware.AuthMiddleware(cfg))
// 	grp.POST("/cover", handlers.UploadCoverImageHandler(pool, svc))
// 	grp.POST("/banner", handlers.UploadBannerImageHandler(pool, svc))
// 	grp.POST("/gallery", handlers.UploadGalleryImageHandler(pool, svc))
// 	return r
// }

// // ─── JWT helper ───────────────────────────────────────────────────────────────

// func makeImgToken(t *testing.T, userID, role, secret string) string {
// 	t.Helper()
// 	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
// 		"user_id": userID,
// 		"role":    role,
// 		"exp":     time.Now().Add(time.Hour).Unix(),
// 	}).SignedString([]byte(secret))
// 	require.NoError(t, err)
// 	return tok
// }

// // ─── DB seed helpers ──────────────────────────────────────────────────────────

// func seedOrganizer(t *testing.T, pool *pgxpool.Pool) string {
// 	t.Helper()
// 	email := fmt.Sprintf("org-%d@test.com", time.Now().UnixNano())
// 	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
// 	require.NoError(t, err)
// 	var id string
// 	err = pool.QueryRow(context.Background(),
// 		`INSERT INTO users (email, password_hash, role, is_verified)
// 		 VALUES ($1, $2, 'organizer', true) RETURNING id`,
// 		email, string(hash),
// 	).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// func seedEvent(t *testing.T, pool *pgxpool.Pool, orgID string, cover, banner *string) string {
// 	t.Helper()
// 	var id string
// 	err := pool.QueryRow(context.Background(),
// 		`INSERT INTO events (organizer_id, title, cover_image_url, banner_image_url)
// 		 VALUES ($1, 'Test Event', $2, $3) RETURNING id`,
// 		orgID, cover, banner,
// 	).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// // ─── multipart body builder ───────────────────────────────────────────────────

// func imgBody(t *testing.T, field, filename, ct string, data []byte) (*bytes.Buffer, string) {
// 	t.Helper()
// 	buf := &bytes.Buffer{}
// 	w := multipart.NewWriter(buf)
// 	h := make(textproto.MIMEHeader)
// 	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
// 	if ct != "" {
// 		h.Set("Content-Type", ct)
// 	}
// 	part, err := w.CreatePart(h)
// 	require.NoError(t, err)
// 	_, err = part.Write(data)
// 	require.NoError(t, err)
// 	require.NoError(t, w.Close())
// 	return buf, w.FormDataContentType()
// }

// // ─── shared fixture: one container + pool for all handler tests ───────────────
// //
// // TestMain would be cleaner but we're in package handlers_test alongside other
// // test files.  Instead we use a package-level sync.Once so the container is
// // started only once per `go test` invocation of this package.

// var (
// 	imgOnce sync.Once
// 	imgPool *pgxpool.Pool
// )

// // imgDB returns the shared pool, starting the container on first call.
// // Each test is responsible for cleaning up its own rows.
// func imgDB(t *testing.T) *pgxpool.Pool {
// 	t.Helper()
// 	// Per-test isolation: spin a fresh container each time so tests can't
// 	// bleed into each other, and the cost is paid once via t.Parallel or
// 	// individually.  For a large suite TestMain is better, but this keeps
// 	// the file self-contained.
// 	return startPostgres(t)
// }

// // ═══════════════════════════════════════════════════════════════════════════════
// // COVER IMAGE HANDLER
// // ═══════════════════════════════════════════════════════════════════════════════

// func TestImage_Cover_HappyPath(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "cover.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/cover", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusOK, w.Code)
// 	var resp map[string]string
// 	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 	assert.Contains(t, resp["cover_image_url"], eventID)
// 	assert.Equal(t, 1, r2.Puts())
// }

// func TestImage_Cover_MissingFile(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	// Multipart body with no "image" field.
// 	buf := &bytes.Buffer{}
// 	mw := multipart.NewWriter(buf)
// 	_ = mw.WriteField("other", "value")
// 	require.NoError(t, mw.Close())

// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/cover", buf)
// 	req.Header.Set("Content-Type", mw.FormDataContentType())
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusBadRequest, w.Code)
// 	assert.Equal(t, 0, r2.Puts())
// }

// func TestImage_Cover_InvalidEventUUID(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "x.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/not-a-uuid/cover", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusBadRequest, w.Code)
// 	assert.Equal(t, 0, r2.Puts(), "no upload should occur for a malformed event ID")
// }

// func TestImage_Cover_EventNotOwned(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	ownerID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, ownerID, nil, nil)
// 	attackerID := seedOrganizer(t, pool)
// 	token := makeImgToken(t, attackerID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "x.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/cover", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusNotFound, w.Code)
// 	assert.Equal(t, 0, r2.Puts())
// }

// func TestImage_Cover_UnsupportedMIME(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "anim.gif", "image/gif", []byte("GIF89a"))
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/cover", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusBadRequest, w.Code)
// 	assert.Equal(t, 0, r2.Puts())
// }

// func TestImage_Cover_FileTooLarge(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	overLimit := make([]byte, 5*1024*1024+1)
// 	overLimit[0], overLimit[1] = 0xFF, 0xD8

// 	body, mime := imgBody(t, "image", "big.jpg", "image/jpeg", overLimit)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/cover", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusBadRequest, w.Code)
// 	assert.Equal(t, 0, r2.Puts())
// }

// func TestImage_Cover_ReplacesExisting(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	existing := fmt.Sprintf("%s/%s/events/old/covers/old.jpg", r2.URL(), cfg.R2Bucket)
// 	eventID := seedEvent(t, pool, orgID, &existing, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "new.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/cover", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	require.Equal(t, http.StatusOK, w.Code)
// 	assert.Equal(t, 1, r2.Puts(), "new cover should be uploaded to R2")
// 	assert.Equal(t, 1, r2.Deletes(), "old cover should be deleted from R2")
// }

// func TestImage_Cover_Unauthenticated(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	body, mime := imgBody(t, "image", "x.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+uuid.New().String()+"/cover", body)
// 	req.Header.Set("Content-Type", mime)
// 	// No Authorization header.
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusUnauthorized, w.Code)
// }

// // ═══════════════════════════════════════════════════════════════════════════════
// // BANNER IMAGE HANDLER
// // ═══════════════════════════════════════════════════════════════════════════════

// func TestImage_Banner_HappyPath(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "banner.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/banner", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusOK, w.Code)
// 	var resp map[string]string
// 	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 	assert.Contains(t, resp["banner_image_url"], eventID)
// }

// func TestImage_Banner_FileTooLarge(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	overLimit := make([]byte, 8*1024*1024+1)
// 	overLimit[0], overLimit[1] = 0xFF, 0xD8

// 	body, mime := imgBody(t, "image", "wide.jpg", "image/jpeg", overLimit)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/banner", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusBadRequest, w.Code)
// 	assert.Equal(t, 0, r2.Puts())
// }

// func TestImage_Banner_ReplacesExisting(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	existing := fmt.Sprintf("%s/%s/events/old/banners/old.jpg", r2.URL(), cfg.R2Bucket)
// 	eventID := seedEvent(t, pool, orgID, nil, &existing)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "new.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/banner", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	require.Equal(t, http.StatusOK, w.Code)
// 	assert.Equal(t, 1, r2.Puts())
// 	assert.Equal(t, 1, r2.Deletes())
// }

// // ═══════════════════════════════════════════════════════════════════════════════
// // GALLERY IMAGE HANDLER
// // ═══════════════════════════════════════════════════════════════════════════════

// func TestImage_Gallery_HappyPath(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "gallery.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/gallery", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	require.Equal(t, http.StatusOK, w.Code)
// 	var resp map[string]string
// 	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 	assert.NotEmpty(t, resp["image_url"])
// }

// func TestImage_Gallery_URLPersistedInDB(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, orgID, nil, nil)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "g.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/gallery", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)
// 	require.Equal(t, http.StatusOK, w.Code)

// 	var resp map[string]string
// 	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

// 	var raw []byte
// 	err := pool.QueryRow(context.Background(),
// 		`SELECT gallery_images FROM events WHERE id = $1`, eventID,
// 	).Scan(&raw)
// 	require.NoError(t, err)
// 	assert.Contains(t, string(raw), resp["image_url"],
// 		"returned URL should be persisted in gallery_images JSONB column")
// }

// func TestImage_Gallery_EventNotOwned(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	ownerID := seedOrganizer(t, pool)
// 	eventID := seedEvent(t, pool, ownerID, nil, nil)
// 	attackerID := seedOrganizer(t, pool)
// 	token := makeImgToken(t, attackerID, "organizer", cfg.JWTSecret)

// 	body, mime := imgBody(t, "image", "g.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID+"/gallery", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusNotFound, w.Code)
// 	assert.Equal(t, 0, r2.Puts())
// }

// func TestImage_Gallery_NonExistentEvent(t *testing.T) {
// 	pool := imgDB(t)
// 	r2 := newR2Mock(t)
// 	cfg := imgHandlerCfg(r2.URL())
// 	svc := services.NewImageService(cfg)

// 	orgID := seedOrganizer(t, pool)
// 	token := makeImgToken(t, orgID, "organizer", cfg.JWTSecret)
// 	ghostID := uuid.New().String()

// 	body, mime := imgBody(t, "image", "g.jpg", "image/jpeg", handlerJPEG)
// 	req := httptest.NewRequest(http.MethodPost, "/events/"+ghostID+"/gallery", body)
// 	req.Header.Set("Content-Type", mime)
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	w := httptest.NewRecorder()
// 	imgRouter(pool, svc, cfg).ServeHTTP(w, req)

// 	assert.Equal(t, http.StatusNotFound, w.Code)
// 	assert.Equal(t, 0, r2.Puts(), "no R2 upload should occur for a non-existent event")
// }

// // ═══════════════════════════════════════════════════════════════════════════════
// // REPOSITORY LAYER — direct unit tests (no HTTP)
// // ═══════════════════════════════════════════════════════════════════════════════

// func TestImageRepo_UpdateCoverImage_HappyPath(t *testing.T) {
// 	pool := imgDB(t)
// 	ctx := context.Background()

// 	var userID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO users (email, password_hash, role, is_verified)
// 		 VALUES ('repo-cover@test.com', 'hash', 'organizer', true) RETURNING id`,
// 	).Scan(&userID))

// 	var eventID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO events (organizer_id, title) VALUES ($1, 'Cover Test') RETURNING id`,
// 		userID,
// 	).Scan(&eventID))

// 	newURL := "https://r2.example.com/bucket/events/id/covers/new.jpg"
// 	require.NoError(t, repository.UpdateEventCoverImage(pool, eventID, userID, newURL))

// 	var stored *string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`SELECT cover_image_url FROM events WHERE id = $1`, eventID,
// 	).Scan(&stored))
// 	require.NotNil(t, stored)
// 	assert.Equal(t, newURL, *stored)
// }

// func TestImageRepo_UpdateCoverImage_WrongOwner(t *testing.T) {
// 	pool := imgDB(t)
// 	ctx := context.Background()

// 	var ownerID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO users (email, password_hash, role, is_verified)
// 		 VALUES ('repo-owner@test.com', 'hash', 'organizer', true) RETURNING id`,
// 	).Scan(&ownerID))

// 	var eventID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO events (organizer_id, title) VALUES ($1, 'Wrong Owner Test') RETURNING id`,
// 		ownerID,
// 	).Scan(&eventID))

// 	err := repository.UpdateEventCoverImage(pool, eventID, uuid.New().String(), "https://cdn.example.com/x.jpg")
// 	assert.ErrorIs(t, err, repository.ErrEventNotFound)
// }

// func TestImageRepo_AppendGallery_Accumulates(t *testing.T) {
// 	pool := imgDB(t)
// 	ctx := context.Background()

// 	var userID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO users (email, password_hash, role, is_verified)
// 		 VALUES ('gallery-acc@test.com', 'hash', 'organizer', true) RETURNING id`,
// 	).Scan(&userID))

// 	var eventID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO events (organizer_id, title) VALUES ($1, 'Gallery Acc') RETURNING id`,
// 		userID,
// 	).Scan(&eventID))

// 	urls := []string{
// 		"https://cdn.example.com/a.jpg",
// 		"https://cdn.example.com/b.jpg",
// 		"https://cdn.example.com/c.jpg",
// 	}
// 	for _, u := range urls {
// 		require.NoError(t, repository.AppendGalleryImage(pool, eventID, userID, u))
// 	}

// 	var raw []byte
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`SELECT gallery_images FROM events WHERE id = $1`, eventID,
// 	).Scan(&raw))
// 	for _, u := range urls {
// 		assert.Contains(t, string(raw), u, "URL %s should appear in gallery_images", u)
// 	}
// }

// func TestImageRepo_GetEventImageURLs_NilWhenEmpty(t *testing.T) {
// 	pool := imgDB(t)
// 	ctx := context.Background()

// 	var userID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO users (email, password_hash, role, is_verified)
// 		 VALUES ('nil-urls@test.com', 'hash', 'organizer', true) RETURNING id`,
// 	).Scan(&userID))

// 	var eventID string
// 	require.NoError(t, pool.QueryRow(ctx,
// 		`INSERT INTO events (organizer_id, title) VALUES ($1, 'No Images') RETURNING id`,
// 		userID,
// 	).Scan(&eventID))

// 	cover, banner, _, _, err := repository.GetEventImageURLs(pool, eventID, userID)
// 	require.NoError(t, err)
// 	assert.Nil(t, cover, "cover should be nil when column is NULL")
// 	assert.Nil(t, banner, "banner should be nil when column is NULL")
// }