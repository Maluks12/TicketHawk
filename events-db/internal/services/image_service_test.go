package services_test

// =============================================================================
// IMAGE SERVICE TESTS
// =============================================================================
//
// Tests covered:
//   ── Unit (mock S3/R2 httptest server, no real credentials needed) ─────────
//   - UploadEventImage: valid JPEG / PNG / WebP MIME types accepted
//   - UploadEventImage: extension fallback when Content-Type header is absent
//   - UploadEventImage: unsupported MIME type (GIF) rejected before any upload
//   - UploadEventImage: unsupported file extension (.bmp) rejected
//   - UploadEventImage: cover size cap  (5 MB) enforced
//   - UploadEventImage: banner size cap (8 MB) enforced
//   - UploadEventImage: gallery size cap (5 MB) enforced
//   - UploadEventImage: banner at exactly 8 MB is accepted
//   - UploadEventImage: object key format  events/<id>/<type>/<uuid>.<ext>
//   - UploadEventImage: successive uploads produce unique keys (UUID-based)
//   - DeleteEventImage: valid URL triggers DELETE on the correct R2 key
//   - DeleteEventImage: foreign-bucket URL rejected without issuing any I/O
//   - DeleteEventImage: empty URL returns an error
//   - Round-trip:        upload → delete removes the object from the mock store
//   - Dead server:       abrupt TCP close surfaces as a wrapped error
//   - ImageType consts:  exact string values are pinned to detect silent renames
//   - Constructor:       NewImageService with zero Config does not panic
//
// Run:
//   go test ./internal/services/... -v -run TestImageService
//   go test ./internal/services/... -v -run TestImageType
//   go test ./internal/services/... -v -run TestNewImageService
// =============================================================================

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"events-db/internal/config"
	"events-db/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── nopFile ─────────────────────────────────────────────────────────────────
// multipart.File requires Read + ReadAt + Seek + Close.
// bytes.Reader provides the first three; we add a no-op Close.

type nopFile struct{ *bytes.Reader }

func (nopFile) Close() error { return nil }

var _ multipart.File = nopFile{} // compile-time interface check

func newFile(data []byte) nopFile { return nopFile{bytes.NewReader(data)} }

// ─── test helpers ─────────────────────────────────────────────────────────────

// fakeHeader builds a *multipart.FileHeader with the given metadata.
// Pass an empty contentType to simulate an absent Content-Type header.
func fakeHeader(size int64, contentType, filename string) *multipart.FileHeader {
	h := make(textproto.MIMEHeader)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	return &multipart.FileHeader{
		Filename: filename,
		Size:     size,
		Header:   h,
	}
}

// imgCfg returns a minimal *config.Config wired to the provided mock R2 URL.
func imgCfg(r2URL string) *config.Config {
	return &config.Config{
		R2AccessKey: "test-access",
		R2SecretKey: "test-secret",
		R2Bucket:    "test-bucket",
		R2URL:       r2URL,
	}
}

// mockR2Server starts an httptest.Server that accepts PutObject (200) and
// DeleteObject (204), discarding all request bodies.
func mockR2Server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// minimal 1×1 JPEG (22 bytes)
var smallJPEG = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
	0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
	0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9,
}

// ═════════════════════════════════════════════════════════════════════════════
// MIME / extension validation
// ═════════════════════════════════════════════════════════════════════════════

func TestImageService_UploadEventImage_ValidJPEG(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	url, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(int64(len(smallJPEG)), "image/jpeg", "photo.jpg"),
		"event-001", services.ImageTypeCover,
	)
	require.NoError(t, err)
	assert.Contains(t, url, "event-001")
	assert.Contains(t, url, string(services.ImageTypeCover))
	assert.True(t, strings.HasSuffix(url, ".jpg"), "URL should end with .jpg, got: %s", url)
}

func TestImageService_UploadEventImage_ValidPNG(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic

	url, err := svc.UploadEventImage(
		newFile(data),
		fakeHeader(int64(len(data)), "image/png", "banner.png"),
		"event-002", services.ImageTypeBanner,
	)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(url, ".png"))
}

func TestImageService_UploadEventImage_ValidWebP(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))
	data := []byte("RIFF????WEBPVP8 ")

	url, err := svc.UploadEventImage(
		newFile(data),
		fakeHeader(int64(len(data)), "image/webp", "thumb.webp"),
		"event-003", services.ImageTypeGallery,
	)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(url, ".webp"))
}

func TestImageService_UploadEventImage_UnsupportedMIME_GIF(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	_, err := svc.UploadEventImage(
		newFile([]byte("GIF89a")),
		fakeHeader(100, "image/gif", "anim.gif"),
		"event-004", services.ImageTypeCover,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file type")
}

func TestImageService_UploadEventImage_UnsupportedExtension_BMP(t *testing.T) {
	// No Content-Type → falls back to extension check.
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	_, err := svc.UploadEventImage(
		newFile([]byte("BM")),
		fakeHeader(100, "", "photo.bmp"),
		"event-005", services.ImageTypeCover,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file type")
}

func TestImageService_UploadEventImage_ExtensionFallback_JPEG(t *testing.T) {
	// Content-Type header absent → service should infer from .jpeg extension.
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	url, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(int64(len(smallJPEG)), "", "photo.jpeg"),
		"event-006", services.ImageTypeCover,
	)
	require.NoError(t, err)
	assert.True(
		t,
		strings.HasSuffix(url, ".jpeg") || strings.HasSuffix(url, ".jpg"),
		"expected a JPEG extension in URL, got: %s", url,
	)
}

// ═════════════════════════════════════════════════════════════════════════════
// Size-limit enforcement
// ═════════════════════════════════════════════════════════════════════════════

func TestImageService_UploadEventImage_CoverOver5MB_Rejected(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	_, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(5*1024*1024+1, "image/jpeg", "big.jpg"),
		"event-007", services.ImageTypeCover,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
	assert.Contains(t, err.Error(), "5", "error should cite the 5 MB limit")
}

func TestImageService_UploadEventImage_BannerOver8MB_Rejected(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	_, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(8*1024*1024+1, "image/jpeg", "wide.jpg"),
		"event-008", services.ImageTypeBanner,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
	assert.Contains(t, err.Error(), "8", "error should cite the 8 MB limit")
}

func TestImageService_UploadEventImage_BannerExactly8MB_Accepted(t *testing.T) {
	// Exactly at the cap — should be allowed.
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	_, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(8*1024*1024, "image/jpeg", "ok.jpg"),
		"event-009", services.ImageTypeBanner,
	)
	require.NoError(t, err, "exactly 8 MB should be accepted for banners")
}

func TestImageService_UploadEventImage_GalleryOver5MB_Rejected(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	_, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(5*1024*1024+1, "image/png", "gallery.png"),
		"event-010", services.ImageTypeGallery,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

// ═════════════════════════════════════════════════════════════════════════════
// Object key path format
// ═════════════════════════════════════════════════════════════════════════════

// TestImageService_KeyPath verifies the S3 key inside the request path is:
//
//	/<bucket>/events/<eventID>/<imageType>/<uuid>.<ext>
func TestImageService_KeyPath(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			capturedPath = r.URL.Path
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	svc := services.NewImageService(imgCfg(srv.URL))
	eventID := "key-path-event"

	_, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(int64(len(smallJPEG)), "image/jpeg", "x.jpg"),
		eventID, services.ImageTypeCover,
	)
	require.NoError(t, err)

	assert.Contains(t, capturedPath, "events/"+eventID)
	assert.Contains(t, capturedPath, string(services.ImageTypeCover))
	assert.True(t, strings.HasSuffix(capturedPath, ".jpg"),
		"key should end with .jpg, got path: %s", capturedPath)
}

// ═════════════════════════════════════════════════════════════════════════════
// UUID uniqueness per upload
// ═════════════════════════════════════════════════════════════════════════════

func TestImageService_UniqueKeysPerUpload(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			paths = append(paths, r.URL.Path)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	svc := services.NewImageService(imgCfg(srv.URL))

	for i := 0; i < 5; i++ {
		_, err := svc.UploadEventImage(
			newFile(smallJPEG),
			fakeHeader(int64(len(smallJPEG)), "image/jpeg", "pic.jpg"),
			"same-event", services.ImageTypeGallery,
		)
		require.NoError(t, err)
	}

	require.Len(t, paths, 5)
	seen := make(map[string]bool)
	for _, p := range paths {
		require.False(t, seen[p], "duplicate object key detected: %s", p)
		seen[p] = true
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// DeleteEventImage
// ═════════════════════════════════════════════════════════════════════════════

func TestImageService_DeleteEventImage_ValidURL(t *testing.T) {
	var deletedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := imgCfg(srv.URL)
	svc := services.NewImageService(cfg)
	key := "events/abc/covers/uuid.jpg"
	publicURL := fmt.Sprintf("%s/%s/%s", srv.URL, cfg.R2Bucket, key)

	err := svc.DeleteEventImage(publicURL)
	require.NoError(t, err)
	assert.Contains(t, deletedPath, key,
		"DELETE request should target the original key")
}

func TestImageService_DeleteEventImage_ForeignURL_NoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	svc := services.NewImageService(imgCfg(srv.URL))
	err := svc.DeleteEventImage("https://evil-cdn.example.com/bucket/file.jpg")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "URL does not belong to this R2 bucket")
	assert.False(t, called, "no HTTP request should be sent for a foreign URL")
}

func TestImageService_DeleteEventImage_EmptyURL(t *testing.T) {
	srv := mockR2Server(t)
	svc := services.NewImageService(imgCfg(srv.URL))

	err := svc.DeleteEventImage("")
	require.Error(t, err)
}

// ═════════════════════════════════════════════════════════════════════════════
// Round-trip: upload then delete
// ═════════════════════════════════════════════════════════════════════════════

func TestImageService_RoundTrip_UploadThenDelete(t *testing.T) {
	stored := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		switch r.Method {
		case http.MethodPut:
			stored[r.URL.Path] = true
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(stored, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)

	svc := services.NewImageService(imgCfg(srv.URL))

	url, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(int64(len(smallJPEG)), "image/jpeg", "pic.jpg"),
		"round-trip-event", services.ImageTypeCover,
	)
	require.NoError(t, err)
	assert.Len(t, stored, 1, "one object should exist after upload")

	err = svc.DeleteEventImage(url)
	require.NoError(t, err)
	assert.Len(t, stored, 0, "object should be gone after delete")
}

// ═════════════════════════════════════════════════════════════════════════════
// Error propagation: dead server
// ═════════════════════════════════════════════════════════════════════════════

func TestImageService_UploadEventImage_DeadServer(t *testing.T) {
	// Server immediately hijacks and closes the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	t.Cleanup(srv.Close)

	svc := services.NewImageService(imgCfg(srv.URL))
	_, err := svc.UploadEventImage(
		newFile(smallJPEG),
		fakeHeader(int64(len(smallJPEG)), "image/jpeg", "x.jpg"),
		"dead-server-event", services.ImageTypeCover,
	)
	require.Error(t, err, "abrupt TCP close should propagate as an upload error")
}

// ═════════════════════════════════════════════════════════════════════════════
// ImageType constant values — pinned to prevent silent URL breakage in R2
// ═════════════════════════════════════════════════════════════════════════════

func TestImageType_Constants_Values(t *testing.T) {
	assert.Equal(t, services.ImageType("covers"), services.ImageTypeCover)
	assert.Equal(t, services.ImageType("banners"), services.ImageTypeBanner)
	// NOTE: "galllery" has an intentional triple-l typo in the source.
	// This test pins the *current* value so that if anyone corrects the
	// spelling they are forced to also decide whether to migrate existing
	// R2 object keys (old keys live at "galllery/", new ones at "gallery/").
	assert.Equal(t, services.ImageType("galllery"), services.ImageTypeGallery)
}

// ═════════════════════════════════════════════════════════════════════════════
// Constructor
// ═════════════════════════════════════════════════════════════════════════════

func TestNewImageService_ZeroConfig_NoPanic(t *testing.T) {
	require.NotPanics(t, func() {
		_ = services.NewImageService(&config.Config{})
	})
}