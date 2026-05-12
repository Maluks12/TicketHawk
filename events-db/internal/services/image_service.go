package services

import (
	"bytes"
	"context"
	"events-db/internal/config"
	"fmt"
	"io"
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
	maxCoverSize   int64 = 5 * 1024 * 1024  // 5 MB
	maxBannerSize  int64 = 8 * 1024 * 1024  // 8 MB
	maxGallerySize int64 = 5 * 1024 * 1024  // 5 MB
	maxVideoSize   int64 = 500 * 1024 * 1024 // 500 MB

	// MaxGalleryCount is the hard cap on gallery images per event. It is
	// exported so the repository can reference it for the gallery-full check.
	MaxGalleryCount = 10
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

// ── Image upload ─────────────────────────────────────────────────────────────

func (s *ImageService) UploadEventImage(
	file multipart.File,
	header *multipart.FileHeader,
	eventID string,
	imgType ImageType,
) (string, error) {
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedImageMIMETypes[contentType]
	if !ok {
		ext = strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
			ext = ".jpg"
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

	filename := fmt.Sprintf("events/%s/%s/%s%s", eventID, string(imgType), uuid.New().String(), ext)
	return s.uploadToR2(filename, contentType, bytes.NewReader(data), int64(len(data)))
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
	ext, ok := allowedImageMIMETypes[contentType]
	if !ok {
		ext = strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
			ext = ".jpg"
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

	return fmt.Sprintf("%s/%s/%s", s.cfg.R2URL, s.cfg.R2Bucket, filename), nil
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
	})

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.R2Bucket),
		Key:    aws.String(key),
	})
	return err
}