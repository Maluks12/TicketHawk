package services

import (
	"bytes"
	"context"
	"events-db/internal/config"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

type ImageService struct {
	cfg *config.Config
}

func NewImageService(cfg *config.Config) *ImageService {
	return &ImageService{cfg: cfg}
}

// ── Media type constants ──────────────────────────────────────────────────────

type ImageType string

const (
	ImageTypeCover          ImageType = "covers"
	ImageTypeBanner         ImageType = "banners"
	ImageTypeGallery        ImageType = "gallery" // fixed typo: was "galllery"
	ImageTypeCollectionCover ImageType = "collection-covers"
)

type VideoType string

const (
	VideoTypeBanner VideoType = "banner_videos"
)

// ── Size and count limits ─────────────────────────────────────────────────────

const (
	maxCoverSize   int64 = 5 * 1024 * 1024   // 5 MB
	maxBannerSize  int64 = 8 * 1024 * 1024   // 8 MB
	maxGallerySize int64 = 5 * 1024 * 1024   // 5 MB
	maxVideoSize   int64 = 500 * 1024 * 1024 // 500 MB

	// MaxGalleryCount is the hard cap on gallery images per event. It is
	// exported so the repository can reference it for the gallery-full check.
	MaxGalleryCount = 10
)

// ── Compression settings ──────────────────────────────────────────────────────

const (
	// maxImageDimension is the maximum width or height of any uploaded image
	// after compression. Images larger than this are scaled down proportionally.
	maxImageDimension = 1920

	// jpegQuality is the JPEG encoding quality (0–100). 82 gives a good
	// balance between visual fidelity and file size (~60–70 % smaller than
	// a typical camera JPEG at 95+ quality).
	jpegQuality = 82
)

// ── Allowed MIME types ────────────────────────────────────────────────────────

var allowedImageMIMETypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var allowedVideoMIMETypes = map[string]string{
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/webm":      ".webm",
}

// ── Image compression ─────────────────────────────────────────────────────────

// compressImage decodes a JPEG or PNG, scales it down if it exceeds
// maxImageDimension on either axis, and re-encodes it.
//
// Output format rules:
//   - JPEG  → always re-encoded as JPEG at jpegQuality.
//   - PNG with transparency → kept as PNG (JPEG has no alpha channel; converting
//     would fill transparent areas with white, breaking logos and merch designs).
//   - PNG without transparency → converted to JPEG for a large size reduction.
//   - WebP  → returned unchanged; Go's stdlib has no WebP decoder and WebP is
//     already an efficient format.
//
// Any decode or encode failure returns an error — the caller must reject the
// upload rather than fall back to the raw bytes, which could be very large.
func compressImage(data []byte, contentType string) ([]byte, string, string, error) {
	// WebP: pass through – already efficient, no stdlib decoder available.
	if contentType == "image/webp" {
		return data, contentType, ".webp", nil
	}

	var (
		img image.Image
		err error
	)

	switch contentType {
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to decode JPEG: %w", err)
		}
	case "image/png":
		img, err = png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to decode PNG: %w", err)
		}
	default:
		return nil, "", "", fmt.Errorf("unsupported image format for compression: %s", contentType)
	}

	// Scale down proportionally if either dimension exceeds the cap.
	img = scaleDown(img, maxImageDimension, maxImageDimension)

	// For PNGs, check whether the image actually uses transparency.
	// If it does, we must keep it as PNG to avoid destroying alpha data.
	if contentType == "image/png" && hasTransparency(img) {
		var buf bytes.Buffer
		if err = png.Encode(&buf, img); err != nil {
			return nil, "", "", fmt.Errorf("failed to encode PNG: %w", err)
		}
		return buf.Bytes(), "image/png", ".png", nil
	}

	// All other cases: re-encode as JPEG.
	// PNG without transparency → JPEG yields 70–90 % size reduction for photos.
	var buf bytes.Buffer
	if err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, "", "", fmt.Errorf("failed to encode JPEG: %w", err)
	}

	return buf.Bytes(), "image/jpeg", ".jpg", nil
}

// hasTransparency reports whether any pixel in img has an alpha value below
// fully opaque. Sampling every pixel is the only reliable method — format
// metadata alone (e.g. PNG color type) does not guarantee pixels are actually
// transparent.
func hasTransparency(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xffff {
				return true
			}
		}
	}
	return false
}

// scaleDown returns img scaled so that neither dimension exceeds maxW/maxH,
// preserving the aspect ratio. Uses bilinear interpolation for quality.
// If the image already fits, img is returned as-is (zero allocation).
func scaleDown(img image.Image, maxW, maxH int) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w <= maxW && h <= maxH {
		return img
	}

	ratio := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	newW := int(math.Round(float64(w) * ratio))
	newH := int(math.Round(float64(h) * ratio))

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	// Bilinear interpolation – good quality for downscaling photos.
	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			// Map destination pixel back to source space.
			srcXf := (float64(x)+0.5)*float64(w)/float64(newW) - 0.5
			srcYf := (float64(y)+0.5)*float64(h)/float64(newH) - 0.5

			x0 := int(math.Floor(srcXf))
			y0 := int(math.Floor(srcYf))
			x1 := x0 + 1
			y1 := y0 + 1

			// Clamp to image bounds.
			x0 = clampInt(x0, bounds.Min.X, bounds.Max.X-1)
			y0 = clampInt(y0, bounds.Min.Y, bounds.Max.Y-1)
			x1 = clampInt(x1, bounds.Min.X, bounds.Max.X-1)
			y1 = clampInt(y1, bounds.Min.Y, bounds.Max.Y-1)

			fx := srcXf - math.Floor(srcXf)
			fy := srcYf - math.Floor(srcYf)

			c00 := toRGBAf(img.At(x0, y0))
			c10 := toRGBAf(img.At(x1, y0))
			c01 := toRGBAf(img.At(x0, y1))
			c11 := toRGBAf(img.At(x1, y1))

			r := lerp2(c00[0], c10[0], c01[0], c11[0], fx, fy)
			g := lerp2(c00[1], c10[1], c01[1], c11[1], fx, fy)
			b := lerp2(c00[2], c10[2], c01[2], c11[2], fx, fy)
			a := lerp2(c00[3], c10[3], c01[3], c11[3], fx, fy)

			dst.Set(x, y, color.RGBA{
				R: uint8(clampFloat(r, 0, 255)),
				G: uint8(clampFloat(g, 0, 255)),
				B: uint8(clampFloat(b, 0, 255)),
				A: uint8(clampFloat(a, 0, 255)),
			})
		}
	}

	return dst
}

// ── Bilinear helpers ──────────────────────────────────────────────────────────

func toRGBAf(c color.Color) [4]float64 {
	r, g, b, a := c.RGBA() // each in [0, 65535]
	return [4]float64{
		float64(r) / 257,
		float64(g) / 257,
		float64(b) / 257,
		float64(a) / 257,
	}
}

func lerp(a, b, t float64) float64 { return a*(1-t) + b*t }

func lerp2(c00, c10, c01, c11, fx, fy float64) float64 {
	return lerp(lerp(c00, c10, fx), lerp(c01, c11, fx), fy)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ── Card thumbnail ────────────────────────────────────────────────────────────

const (
	thumbMaxDimension = 600 // longest edge of the card thumbnail
	thumbJPEGQuality  = 80
)

// makeThumbnail produces a small JPEG (max 600px) used on list/feed cards.
// Transparency is flattened onto white so PNGs don't turn black in JPEG.
// WebP is unsupported by the stdlib decoder and returns an error — the caller
// treats that as "no thumbnail" (the app falls back to the full image).
func makeThumbnail(data []byte, contentType string) ([]byte, error) {
	var (
		img image.Image
		err error
	)
	switch contentType {
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
	case "image/png":
		img, err = png.Decode(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("thumbnail unsupported for content type %s", contentType)
	}
	if err != nil {
		return nil, fmt.Errorf("decode for thumbnail: %w", err)
	}

	img = scaleDown(img, thumbMaxDimension, thumbMaxDimension)

	b := img.Bounds()
	flat := image.NewRGBA(b)
	draw.Draw(flat, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, b, img, b.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: thumbJPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w", err)
	}
	return buf.Bytes(), nil
}

func extensionForMIME(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

// ── Image upload ─────────────────────────────────────────────────────────────

func (s *ImageService) UploadEventImage(
	file multipart.File,
	header *multipart.FileHeader,
	eventID string,
	imgType ImageType,
) (string, error) {
	contentType := header.Header.Get("Content-Type")
	_, ok := allowedImageMIMETypes[contentType]
	if !ok {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".webp":
			contentType = "image/webp"
		default:
			return "", fmt.Errorf("unsupported file type: only JPEG, PNG, and WebP are allowed")
		}
	}

	var maxSize int64
	switch imgType {
	case ImageTypeCover:
		maxSize = maxCoverSize
	case ImageTypeBanner:
		maxSize = maxBannerSize
	default:
		maxSize = maxGallerySize
	}

	if header.Size > maxSize {
		return "", fmt.Errorf("file too large: maximum size is %d MB", maxSize/1024/1024)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Keep the original bytes/type so the thumbnail is generated from the source
	// rather than the already-recompressed full image.
	rawData := data
	origContentType := contentType

	// Compress before uploading.
	data, contentType, ext, err := compressImage(data, contentType)
	if err != nil {
		return "", fmt.Errorf("image compression failed: %w", err)
	}

	filename := fmt.Sprintf("events/%s/%s/%s%s", eventID, string(imgType), uuid.New().String(), ext)
	fullURL, err := s.uploadToR2(filename, contentType, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	// Best-effort card thumbnail for covers & banners, stored at a deterministic
	// sibling path ("<key>_thumb.jpg") so the app derives its URL with no DB or
	// query changes. Failure here never fails the upload.
	if imgType == ImageTypeCover || imgType == ImageTypeBanner {
		if thumb, terr := makeThumbnail(rawData, origContentType); terr == nil {
			thumbKey := strings.TrimSuffix(filename, ext) + "_thumb.jpg"
			_, _ = s.uploadToR2(thumbKey, "image/jpeg", bytes.NewReader(thumb), int64(len(thumb)))
		}
	}

	return fullURL, nil
}

// ── Collection cover upload ───────────────────────────────────────────────────

// UploadCollectionImage validates and uploads a cover image for a merch collection.
// Files are stored at collections/{collectionID}/cover/{uuid}.ext
func (s *ImageService) UploadCollectionImage(
	file multipart.File,
	header *multipart.FileHeader,
	collectionID string,
) (string, error) {
	contentType := header.Header.Get("Content-Type")
	_, ok := allowedImageMIMETypes[contentType]
	if !ok {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".webp":
			contentType = "image/webp"
		default:
			return "", fmt.Errorf("unsupported file type: only JPEG, PNG, and WebP are allowed")
		}
	}

	if header.Size > maxBannerSize {
		return "", fmt.Errorf("file too large: maximum size is %d MB", maxBannerSize/1024/1024)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Compress before uploading.
	data, contentType, ext, err := compressImage(data, contentType)
	if err != nil {
		return "", fmt.Errorf("image compression failed: %w", err)
	}

	filename := fmt.Sprintf("collections/%s/cover/%s%s", collectionID, uuid.New().String(), ext)
	return s.uploadToR2(filename, contentType, bytes.NewReader(data), int64(len(data)))
}

// ── Video upload ─────────────────────────────────────────────────────────────

// UploadEventVideo validates and streams a video file to R2 without loading it
// fully into memory. This is important because banner videos can be up to 500 MB —
// the load-all approach used for images would exhaust memory under concurrent uploads.
func (s *ImageService) UploadEventVideo(
	file multipart.File,
	header *multipart.FileHeader,
	eventID string,
	vidType VideoType,
) (string, error) {
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedVideoMIMETypes[contentType]
	if !ok {
		ext = strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".mp4":
			contentType = "video/mp4"
		case ".mov":
			contentType = "video/quicktime"
		case ".webm":
			contentType = "video/webm"
		default:
			return "", fmt.Errorf("unsupported video format: only MP4, MOV, and WebM are allowed")
		}
	}

	if header.Size > maxVideoSize {
		return "", fmt.Errorf("video too large: maximum size is %d MB", maxVideoSize/1024/1024)
	}

	filename := fmt.Sprintf("events/%s/%s/%s%s", eventID, string(vidType), uuid.New().String(), ext)

	// Stream directly from the multipart file — do not ReadAll. The S3 PutObject
	// call accepts any io.Reader; passing header.Size as ContentLength lets R2
	// validate the transfer without requiring a seekable body.
	return s.uploadToR2(filename, contentType, file, header.Size)
}

// ── R2 primitives ─────────────────────────────────────────────────────────────

// uploadToR2 is the single upload primitive used by both images and videos.
// Accepting an io.Reader + explicit content length means callers choose whether
// to buffer (images) or stream (videos) without duplicating the S3 wiring.
func (s *ImageService) uploadToR2(filename, contentType string, body io.Reader, contentLength int64) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.cfg.R2AccessKey, s.cfg.R2SecretKey, "",
		)),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to load R2 config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s.cfg.R2URL)
		o.UsePathStyle = true
	})

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.cfg.R2Bucket),
		Key:           aws.String(filename),
		Body:          body,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(contentLength),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to R2: %w", err)
	}

	// Use R2_PUBLIC_URL (e.g. https://pub-xxx.r2.dev) for the returned URL.
	// This is the public CDN endpoint — distinct from the S3 API endpoint.
	// Falls back to the S3 path-style URL if R2_PUBLIC_URL is not set.
	base := strings.TrimRight(s.cfg.R2PublicURL, "/")
	if base == "" {
		base = fmt.Sprintf("%s/%s", strings.TrimRight(s.cfg.R2URL, "/"), s.cfg.R2Bucket)
	}
	return fmt.Sprintf("%s/%s", base, filename), nil
}

// DeleteEventImage removes any event media file (image or video) from R2 by
// its public URL. The function name is kept for backwards compatibility.
func (s *ImageService) DeleteEventImage(publicURL string) error {
	prefix := fmt.Sprintf("%s/%s/", s.cfg.R2URL, s.cfg.R2Bucket)
	if !strings.HasPrefix(publicURL, prefix) {
		return fmt.Errorf("URL does not belong to this R2 bucket")
	}

	key := strings.TrimPrefix(publicURL, prefix)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.cfg.R2AccessKey, s.cfg.R2SecretKey, "",
		)),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return fmt.Errorf("failed to load R2 config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s.cfg.R2URL)
		o.UsePathStyle = true
	})

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.R2Bucket),
		Key:    aws.String(key),
	})
	return err
}

// UploadGenericImage stores an image under uploads/misc/{uuid}.ext and returns
// the public URL. No DB write — caller associates the URL wherever needed.
// Used by the admin dashboard for merch images during both create and edit flows.
func (s *ImageService) UploadGenericImage(
	file multipart.File,
	header *multipart.FileHeader,
) (string, error) {
	contentType := header.Header.Get("Content-Type")
	_, ok := allowedImageMIMETypes[contentType]
	if !ok {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".webp":
			contentType = "image/webp"
		default:
			return "", fmt.Errorf("unsupported file type: only JPEG, PNG, and WebP are allowed")
		}
	}

	const maxSize int64 = 10 * 1024 * 1024 // 10 MB
	if header.Size > maxSize {
		return "", fmt.Errorf("file too large: maximum size is 10 MB")
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Compress before uploading.
	data, contentType, ext, err := compressImage(data, contentType)
	if err != nil {
		return "", fmt.Errorf("image compression failed: %w", err)
	}

	filename := fmt.Sprintf("uploads/misc/%s%s", uuid.New().String(), ext)
	return s.uploadToR2(filename, contentType, bytes.NewReader(data), int64(len(data)))
}