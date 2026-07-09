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

// scaleDown returns img scaled so that neither dimension exceeds maxW/maxH,
// preserving the aspect ratio. Uses area averaging (box filter) — the correct
// filter for large downscale factors, since every source pixel contributes to
// the output instead of most being skipped (which is what bilinear does at
// ratios beyond ~2x, causing aliasing and muddy thumbnails).
// Mirrors internal/services/image_service.go.
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
		sy0 := float64(y) * yRatio
		sy1 := sy0 + yRatio
		iy0 := int(sy0)
		iy1 := int(math.Ceil(sy1))
		if iy1 > h {
			iy1 = h
		}
		for x := 0; x < newW; x++ {
			sx0 := float64(x) * xRatio
			sx1 := sx0 + xRatio
			ix0 := int(sx0)
			ix1 := int(math.Ceil(sx1))
			if ix1 > w {
				ix1 = w
			}

			var rSum, gSum, bSum, aSum, cov float64
			for sy := iy0; sy < iy1; sy++ {
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