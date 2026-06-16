package handlers_test

// =============================================================================
// THUMBNAIL GENERATION TESTS
// =============================================================================
//
// Verifies the card-thumbnail variant feature: when a cover/banner is uploaded,
// the image service also writes a deterministic "<key>_thumb.jpg" sibling to R2,
// downscaled to <= 600px. The app derives that URL from the stored cover/banner
// URL, so no DB columns or query changes are involved.
//
// Like the other handler tests in this package, these spin up against a real
// Postgres started via Docker. R2 is faked with an in-process httptest server,
// so no Cloudflare credentials are needed.
//
// Prerequisites -- start the test database once before running:
//
//	docker run -d --name test-postgres \
//	  -e POSTGRES_USER=testuser \
//	  -e POSTGRES_PASSWORD=testpass \
//	  -e POSTGRES_DB=test_db \
//	  -p 5433:5432 postgres:15
//
// Run:
//
//	go test ./internal/handlers/... -v -run TestThumbnail
//
// =============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Mock R2 (in-process S3) ──────────────────────────────────────────────────

type mockR2 struct {
	mu      sync.Mutex
	objects map[string][]byte
	bucket  string
	srv     *httptest.Server
}

func newMockR2(t *testing.T, bucket string) *mockR2 {
	t.Helper()
	m := &mockR2{objects: map[string][]byte{}, bucket: bucket}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/"+bucket+"/")
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			m.mu.Lock()
			m.objects[key] = body
			m.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			m.mu.Lock()
			body, ok := m.objects[key]
			m.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodDelete:
			m.mu.Lock()
			delete(m.objects, key)
			m.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockR2) get(key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	return b, ok
}

func (m *mockR2) keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.objects))
	for k := range m.objects {
		out = append(out, k)
	}
	return out
}

// ─── Test harness ─────────────────────────────────────────────────────────────

func thumbTestConfig(r2 *mockR2) *config.Config {
	return &config.Config{
		JWTSecret:   "test-secret-that-is-at-least-32-chars!!",
		R2AccessKey: "test-access",
		R2SecretKey: "test-secret",
		R2Bucket:    r2.bucket,
		R2URL:       r2.srv.URL,
		R2PublicURL: r2.srv.URL + "/" + r2.bucket,
	}
}

// setupThumbTestDB reuses the package's shared Docker Postgres (authTestDBURL),
// creating only the minimal users/events schema the image handlers touch.
func setupThumbTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("thumbnail tests skipped -- could not create pool: %v\n"+
			"Start it with:\n"+
			"  docker run -d --name test-postgres"+
			" -e POSTGRES_USER=testuser"+
			" -e POSTGRES_PASSWORD=testpass"+
			" -e POSTGRES_DB=test_db"+
			" -p 5433:5432 postgres:15", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("thumbnail tests skipped -- DB not reachable: %v", pingErr)
	}

	ctx := context.Background()
	stmts := []string{
		`DROP TABLE IF EXISTS events CASCADE`,
		`DROP TABLE IF EXISTS users  CASCADE`,
		`CREATE TABLE users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			role       VARCHAR(50)  NOT NULL DEFAULT 'attendee',
			created_at TIMESTAMPTZ  DEFAULT NOW()
		)`,
		`CREATE TABLE events (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			cover_image_url  TEXT,
			banner_image_url TEXT,
			banner_video_url TEXT,
			gallery_images   JSONB DEFAULT '[]'::jsonb,
			updated_at       TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			pool.Close()
			t.Fatalf("schema setup failed: %v\nstmt: %s", err, s)
		}
	}
	t.Cleanup(pool.Close)
	return pool
}

func buildThumbRouter(pool *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	imgService := services.NewImageService(cfg)
	r := gin.New()
	media := r.Group("/v1/organiser/events/:id/media")
	media.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	media.POST("/cover", handlers.UploadCoverImageHandler(pool, imgService))
	media.POST("/banner", handlers.UploadBannerImageHandler(pool, imgService))
	return r
}

func thumbToken(t *testing.T, userID, role, secret string) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	require.NoError(t, err)
	return tok
}

func seedThumbEvent(t *testing.T, pool *pgxpool.Pool) (orgID, eventID string) {
	t.Helper()
	ctx := context.Background()
	email := fmt.Sprintf("org-%d@test.com", time.Now().UnixNano())
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (email, role) VALUES ($1, 'organizer') RETURNING id`, email,
	).Scan(&orgID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO events (organizer_id) VALUES ($1) RETURNING id`, orgID,
	).Scan(&eventID))
	return orgID, eventID
}

// makeTestJPEG builds a real JPEG of the given size so the thumbnail path
// actually decodes and downscales it.
func makeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func uploadMedia(t *testing.T, r *gin.Engine, token, eventID, kind, filename, contentType string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename))
	if contentType != "" {
		hdr.Set("Content-Type", contentType)
	}
	part, err := mw.CreatePart(hdr)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost,
		"/v1/organiser/events/"+eventID+"/media/"+kind, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// keyFromPublicURL strips the public base, yielding the R2 object key.
func keyFromPublicURL(u, publicBase string) string {
	return strings.TrimPrefix(u, strings.TrimRight(publicBase, "/")+"/")
}

// deriveThumbKey mirrors the deterministic rule both the backend and the app use:
// replace the extension with "_thumb.jpg".
func deriveThumbKey(fullKey string) string {
	dot := strings.LastIndex(fullKey, ".")
	slash := strings.LastIndex(fullKey, "/")
	if dot <= slash {
		return ""
	}
	return fullKey[:dot] + "_thumb.jpg"
}

// ═════════════════════════════════════════════════════════════════════════════
// Tests
// ═════════════════════════════════════════════════════════════════════════════

func TestThumbnail_CoverUpload_GeneratesThumbnail(t *testing.T) {
	assertVariantUpload(t, "cover", "cover_image_url", "cover_image_url")
}

func TestThumbnail_BannerUpload_GeneratesThumbnail(t *testing.T) {
	assertVariantUpload(t, "banner", "banner_image_url", "banner_image_url")
}

// assertVariantUpload runs the full happy-path for a cover or banner upload.
func assertVariantUpload(t *testing.T, kind, respKey, dbColumn string) {
	pool := setupThumbTestDB(t)
	r2 := newMockR2(t, "test-bucket")
	cfg := thumbTestConfig(r2)
	router := buildThumbRouter(pool, cfg)

	orgID, eventID := seedThumbEvent(t, pool)
	token := thumbToken(t, orgID, "organizer", cfg.JWTSecret)

	src := makeTestJPEG(t, 1200, 800)
	w := uploadMedia(t, router, token, eventID, kind, kind+".jpg", "image/jpeg", src)
	require.Equal(t, http.StatusOK, w.Code, "unexpected status; body: %s", w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	fullURL, _ := resp[respKey].(string)
	require.NotEmpty(t, fullURL, "%s missing in response", respKey)

	// Full URL persisted to the right column.
	var dbURL *string
	require.NoError(t, pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT %s FROM events WHERE id=$1`, dbColumn), eventID).Scan(&dbURL))
	require.NotNil(t, dbURL)
	assert.Equal(t, fullURL, *dbURL)

	// Both the full image and its derived thumbnail were written to R2.
	fullKey := keyFromPublicURL(fullURL, cfg.R2PublicURL)
	thumbKey := deriveThumbKey(fullKey)
	require.NotEmpty(t, thumbKey)

	fullBytes, okFull := r2.get(fullKey)
	require.True(t, okFull, "full object missing at %s; keys=%v", fullKey, r2.keys())
	thumbBytes, okThumb := r2.get(thumbKey)
	require.True(t, okThumb, "thumbnail missing at %s; keys=%v", thumbKey, r2.keys())

	// Thumbnail is a valid JPEG, downscaled to a 600px long edge, and smaller.
	thumbImg, err := jpeg.Decode(bytes.NewReader(thumbBytes))
	require.NoError(t, err, "thumbnail should decode as JPEG")
	tb := thumbImg.Bounds()
	assert.LessOrEqual(t, tb.Dx(), 600)
	assert.LessOrEqual(t, tb.Dy(), 600)
	assert.Equal(t, 600, max(tb.Dx(), tb.Dy()),
		"a 1200x800 source should scale to a 600px long edge")
	assert.Less(t, len(thumbBytes), len(fullBytes),
		"thumbnail should be smaller than the full image")
}

// WebP can't be decoded by the stdlib, so no thumbnail is produced — the upload
// still succeeds and the app falls back to the full image.
func TestThumbnail_WebPUpload_SkipsThumbnail(t *testing.T) {
	pool := setupThumbTestDB(t)
	r2 := newMockR2(t, "test-bucket")
	cfg := thumbTestConfig(r2)
	router := buildThumbRouter(pool, cfg)

	orgID, eventID := seedThumbEvent(t, pool)
	token := thumbToken(t, orgID, "organizer", cfg.JWTSecret)

	webp := []byte("RIFF????WEBPVP8 placeholder-bytes-not-decoded")
	w := uploadMedia(t, router, token, eventID, "cover", "cover.webp", "image/webp", webp)
	require.Equal(t, http.StatusOK, w.Code, "unexpected status; body: %s", w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	fullURL, _ := resp["cover_image_url"].(string)
	require.True(t, strings.HasSuffix(fullURL, ".webp"), "expected .webp URL, got %s", fullURL)

	keys := r2.keys()
	require.Len(t, keys, 1, "webp upload should store exactly one object; keys=%v", keys)
	assert.NotContains(t, keys[0], "_thumb", "no thumbnail object expected for webp")
}

// The derivation rule is the contract the mobile app relies on; pin it.
func TestThumbnail_KeyDerivation_Contract(t *testing.T) {
	cases := []struct{ in, want string }{
		{"events/abc/covers/uuid.jpg", "events/abc/covers/uuid_thumb.jpg"},
		{"events/abc/banners/uuid.png", "events/abc/banners/uuid_thumb.jpg"},
		{"events/abc/covers/uuid.jpeg", "events/abc/covers/uuid_thumb.jpg"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, deriveThumbKey(c.in), "in=%s", c.in)
	}
}