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

	// minHeroLongEdge is the minimum long-edge (px) accepted for covers and
	// banners. Both render as the full-screen hero on the event detail page,
	// which needs ~1080–1980 physical pixels on modern phones — anything much
	// smaller upscales into visible pixelation. 1000px keeps the bar practical
	// for organizers while ruling out card-sized images.
	minHeroLongEdge = 1000
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
// preserving the aspect ratio. If the image already fits, img is returned
// as-is (zero allocation).
//
// Uses area averaging (a box filter): every source pixel inside the footprint
// of an output pixel contributes proportionally to its coverage. This is the
// correct filter for large downscale factors (e.g. a 5000px camera photo down
// to a 600px thumbnail). Bilinear, which this replaces, samples only the 4
// source pixels nearest each output pixel, so at ratios beyond ~2x most of the
// source is skipped entirely — producing aliasing, shimmer, and muddy detail.
func scaleDown(img image.Image, maxW, maxH int) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w <= maxW && h <= maxH {
		return img
	}

	ratio := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	newW := int(math.Round(float64(w) * ratio))
	newH := int(math.Round(float64(h) * ratio))
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	// Normalise the source to a zero-origin RGBA buffer for fast, direct
	// pixel access (avoids the interface-call cost of img.At per sample).
	src, ok := img.(*image.RGBA)
	if !ok || src.Rect.Min != (image.Point{}) {
		tmp := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(tmp, tmp.Bounds(), img, bounds.Min, draw.Src)
		src = tmp
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xRatio := float64(w) / float64(newW)
	yRatio := float64(h) / float64(newH)

	for y := 0; y < newH; y++ {
		// Vertical footprint of this output row in source space.
		sy0 := float64(y) * yRatio
		sy1 := sy0 + yRatio
		iy0 := int(sy0)
		iy1 := int(math.Ceil(sy1))
		if iy1 > h {
			iy1 = h
		}

		for x := 0; x < newW; x++ {
			// Horizontal footprint of this output column in source space.
			sx0 := float64(x) * xRatio
			sx1 := sx0 + xRatio
			ix0 := int(sx0)
			ix1 := int(math.Ceil(sx1))
			if ix1 > w {
				ix1 = w
			}

			var rSum, gSum, bSum, aSum, cov float64
			for sy := iy0; sy < iy1; sy++ {
				// Fraction of this source row covered by the footprint.
				cy := math.Min(sy1, float64(sy+1)) - math.Max(sy0, float64(sy))
				rowOff := sy * src.Stride
				for sx := ix0; sx < ix1; sx++ {
					cx := math.Min(sx1, float64(sx+1)) - math.Max(sx0, float64(sx))
					c := cx * cy
					off := rowOff + sx*4
					rSum += float64(src.Pix[off]) * c
					gSum += float64(src.Pix[off+1]) * c
					bSum += float64(src.Pix[off+2]) * c
					aSum += float64(src.Pix[off+3]) * c
					cov += c
				}
			}

			off := y*dst.Stride + x*4
			dst.Pix[off] = uint8(rSum/cov + 0.5)
			dst.Pix[off+1] = uint8(gSum/cov + 0.5)
			dst.Pix[off+2] = uint8(bSum/cov + 0.5)
			dst.Pix[off+3] = uint8(aSum/cov + 0.5)
		}
	}

	return dst
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

	// Reject covers/banners that are too small to render sharply as the
	// full-screen hero on the event detail page. DecodeConfig reads only the
	// header — no full decode. WebP has no stdlib decoder, so it is skipped
	// here (compressImage passes WebP through untouched anyway).
	if imgType == ImageTypeCover || imgType == ImageTypeBanner {
		if imgCfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(data)); cfgErr == nil {
			longEdge := imgCfg.Width
			if imgCfg.Height > longEdge {
				longEdge = imgCfg.Height
			}
			if longEdge < minHeroLongEdge {
				return "", fmt.Errorf(
					"image too small: covers and banners must be at least %dpx on the longest side (got %dx%d) — small images look pixelated on the event page",
					minHeroLongEdge, imgCfg.Width, imgCfg.Height,
				)
			}
		}
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

	// Keep the ORIGINAL bytes and type for the thumbnail, before compression
	// rewrites them. makeThumbnail scales from the source for the same reason
	// UploadEventImage does: thumbnailing an already-compressed JPEG stacks two
	// lossy passes and the artefacts show at small sizes.
	rawData, origContentType := data, contentType

	// Compress before uploading.
	data, contentType, ext, err := compressImage(data, contentType)
	if err != nil {
		return "", fmt.Errorf("image compression failed: %w", err)
	}

	filename := fmt.Sprintf("uploads/misc/%s%s", uuid.New().String(), ext)
	fullURL, err := s.uploadToR2(filename, contentType, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	// ── Card thumbnail ──────────────────────────────────────────────────────
	//
	// Mirrors the block in UploadEventImage, at the same deterministic sibling
	// path ("<key>_thumb.jpg"), so thumbUrl() on the client derives it with no
	// DB field and no query change.
	//
	// This path is not just organiser logos. POST /organiser/upload is this
	// function, which means LINE-UP images come through here too — those render
	// at 46pt in the participant rows, and an organiser logo renders at 42pt in
	// the ORGANISED BY row of every one of that organiser's events. Serving a
	// full-resolution image into a 42pt box was a download and a decode per
	// screen open.
	//
	// Best-effort: a thumbnail failure must never fail the upload the organiser
	// is waiting on. CoverImage falls back to the full URL when the thumbnail
	// 404s, so the worst case is exactly the behaviour before this block
	// existed.
	if thumb, terr := makeThumbnail(rawData, origContentType); terr == nil {
		thumbKey := strings.TrimSuffix(filename, ext) + "_thumb.jpg"
		_, _ = s.uploadToR2(thumbKey, "image/jpeg", bytes.NewReader(thumb), int64(len(thumb)))
	}

	return fullURL, nil
}