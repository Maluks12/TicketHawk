// Command backfill-thumbs generates the card-sized "_thumb.jpg" variant for
// every existing event cover/banner image that doesn't already have one.
//
// It is idempotent: images that already have a thumbnail are skipped, so it is
// safe to run more than once. WebP originals are skipped (Go's stdlib can't
// decode them) — the app falls back to the full image for those.
//
//	Run from the events-db module root with the same env as the API:
//	  go run ./cmd/backfill-thumbs
//
// Place this file at: events-db/cmd/backfill-thumbs/main.go
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"strings"
	"time"

	"events-db/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	thumbMaxDimension = 600
	thumbJPEGQuality  = 80
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	client, err := newR2Client(ctx, cfg)
	if err != nil {
		log.Fatalf("r2 client: %v", err)
	}

	publicBase := strings.TrimRight(cfg.R2PublicURL, "/")
	if publicBase == "" {
		publicBase = fmt.Sprintf("%s/%s", strings.TrimRight(cfg.R2URL, "/"), cfg.R2Bucket)
	}

	rows, err := pool.Query(ctx, `
		SELECT cover_image_url, banner_image_url
		FROM   events
		WHERE  cover_image_url IS NOT NULL OR banner_image_url IS NOT NULL
	`)
	if err != nil {
		log.Fatalf("query events: %v", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var cover, banner *string
		if err := rows.Scan(&cover, &banner); err != nil {
			log.Fatalf("scan: %v", err)
		}
		if cover != nil && *cover != "" {
			urls = append(urls, *cover)
		}
		if banner != nil && *banner != "" {
			urls = append(urls, *banner)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}

	var made, skipped, failed int
	for _, u := range urls {
		key := keyFromURL(u, publicBase)
		if key == "" {
			skipped++
			continue
		}
		ext := lowerExt(key)
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			// WebP or unknown — stdlib can't make a thumb; app falls back to full.
			skipped++
			continue
		}
		thumbKey := strings.TrimSuffix(key, ext) + "_thumb.jpg"

		if objectExists(ctx, client, cfg.R2Bucket, thumbKey) {
			skipped++
			continue
		}

		full, ct, err := getObject(ctx, client, cfg.R2Bucket, key)
		if err != nil {
			log.Printf("get %s: %v", key, err)
			failed++
			continue
		}
		if ct == "" {
			ct = contentTypeForExt(ext)
		}
		thumb, err := makeThumbnail(full, ct)
		if err != nil {
			log.Printf("thumb %s: %v", key, err)
			failed++
			continue
		}
		if err := putObject(ctx, client, cfg.R2Bucket, thumbKey, "image/jpeg", thumb); err != nil {
			log.Printf("put %s: %v", thumbKey, err)
			failed++
			continue
		}
		made++
		if made%25 == 0 {
			log.Printf("...%d thumbnails created", made)
		}
	}

	log.Printf("done: %d created, %d skipped, %d failed (of %d images)", made, skipped, failed, len(urls))
}

// ── URL/key helpers ───────────────────────────────────────────────────────────

func keyFromURL(u, publicBase string) string {
	if !strings.HasPrefix(u, publicBase+"/") {
		return ""
	}
	return strings.TrimPrefix(u, publicBase+"/")
}

func lowerExt(key string) string {
	dot := strings.LastIndex(key, ".")
	slash := strings.LastIndex(key, "/")
	if dot <= slash {
		return ""
	}
	return strings.ToLower(key[dot:])
}

func contentTypeForExt(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	default:
		return "image/jpeg"
	}
}

// ── Thumbnail ─────────────────────────────────────────────────────────────────

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
		return nil, fmt.Errorf("unsupported content type %s", contentType)
	}
	if err != nil {
		return nil, err
	}

	img = scaleDown(img, thumbMaxDimension, thumbMaxDimension)

	b := img.Bounds()
	flat := image.NewRGBA(b)
	draw.Draw(flat, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, b, img, b.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: thumbJPEGQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

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
	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			srcXf := (float64(x)+0.5)*float64(w)/float64(newW) - 0.5
			srcYf := (float64(y)+0.5)*float64(h)/float64(newH) - 0.5
			x0 := int(math.Floor(srcXf))
			y0 := int(math.Floor(srcYf))
			x1 := x0 + 1
			y1 := y0 + 1
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
			bb := lerp2(c00[2], c10[2], c01[2], c11[2], fx, fy)
			a := lerp2(c00[3], c10[3], c01[3], c11[3], fx, fy)
			dst.Set(x, y, color.RGBA{
				R: uint8(clampFloat(r, 0, 255)),
				G: uint8(clampFloat(g, 0, 255)),
				B: uint8(clampFloat(bb, 0, 255)),
				A: uint8(clampFloat(a, 0, 255)),
			})
		}
	}
	return dst
}

func toRGBAf(c color.Color) [4]float64 {
	r, g, b, a := c.RGBA()
	return [4]float64{float64(r) / 257, float64(g) / 257, float64(b) / 257, float64(a) / 257}
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

// ── R2 primitives ─────────────────────────────────────────────────────────────

func newR2Client(ctx context.Context, cfg *config.Config) (*s3.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.R2AccessKey, cfg.R2SecretKey, "",
		)),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.R2URL)
		o.UsePathStyle = true
	}), nil
}

func objectExists(ctx context.Context, c *s3.Client, bucket, key string) bool {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err := c.HeadObject(cctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err == nil
}

func getObject(ctx context.Context, c *s3.Client, bucket, key string) ([]byte, string, error) {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := c.GetObject(cctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", err
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, "", err
	}
	ct := ""
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	return data, ct, nil
}

func putObject(ctx context.Context, c *s3.Client, bucket, key, contentType string, data []byte) error {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_, err := c.PutObject(cctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(data))),
	})
	return err
}