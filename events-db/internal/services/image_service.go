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

type ImageType string

const (
	ImageTypeCover   ImageType = "covers"
	ImageTypeBanner  ImageType = "banners"
	ImageTypeGallery ImageType = "galllery"
)

const (
	maxCoverSize    = 5 * 1024 * 1024 // 5mb
	maxBannerSize   = 8 * 1024 * 1024 //8mb
	maxGallerySize  = 5 * 1024 * 1024 //5mb
	maxgalleryCount = 10              // could change after discussion
)

var allowedMIMETypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

func (s *ImageService) UploadEventImage(
	file multipart.File,
	header *multipart.FileHeader,
	eventID string,
	imgType ImageType,
) (string, error) {
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedMIMETypes[contentType]
	if !ok {
		ext = strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			return "", fmt.Errorf("unsupported file type: only JPEG, PNG, and WebP are allowed")
		}

		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".webp":
			contentType = "image/webp"
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

	return s.uploadToR2(filename, contentType, data)
}

func (s *ImageService) uploadToR2(filename string, contentType string, data []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5 *time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s.cfg.R2AccessKey, s.cfg.R2SecretKey, "")), awsconfig.WithRegion("auto"))
	if err != nil {
		return "", fmt.Errorf("failed to load R2 config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options){
		o.BaseEndpoint = aws.String(s.cfg.R2URL)
	}) 
		
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.cfg.R2Bucket),
		Key: aws.String(filename),
		Body: bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload to R2: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", s.cfg.R2URL, s.cfg.R2Bucket, filename), nil
}

func (s *ImageService) DeleteEventImage(publicURL string) error {
	prefix := fmt.Sprintf("%s/%s/", s.cfg.R2URL, s.cfg.R2Bucket)
	if !strings.HasPrefix(publicURL, prefix) {
		return fmt.Errorf("URL does not belong to this R2 bucket")
	}

	key := strings.TrimPrefix(publicURL, prefix)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s.cfg.R2AccessKey, s.cfg.R2SecretKey, "")),awsconfig.WithRegion("auto"))
	if err != nil {
		return fmt.Errorf("failed to load R2 config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options){
		o.BaseEndpoint = aws.String(s.cfg.R2URL)
	})

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.R2Bucket),
		Key: aws.String(key),
	})

	return err
}
