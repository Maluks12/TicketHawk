package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrGalleryFull is returned when an organiser tries to add an image to a
// gallery that has already reached the MaxGalleryCount limit.
var ErrGalleryFull = errors.New("gallery is full: maximum 10 images per event")

func UpdateEventCoverImage(pool *pgxpool.Pool, eventID, organizerID, imageURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET    cover_image_url = $1,
		       updated_at      = CURRENT_TIMESTAMP
		WHERE  id = $2 AND organizer_id = $3
	`, imageURL, eventID, organizerID)
	if err != nil {
		return fmt.Errorf("failed to update cover image: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

func UpdateEventBannerImage(pool *pgxpool.Pool, eventID, organizerID, imageURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET    banner_image_url = $1,
		       updated_at       = CURRENT_TIMESTAMP
		WHERE  id = $2 AND organizer_id = $3
	`, imageURL, eventID, organizerID)
	if err != nil {
		return fmt.Errorf("failed to update banner image: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

// UpdateEventBannerVideo stores (or replaces) the banner video URL for an event.
// Pass an empty string to clear the video — it will be stored as NULL.
func UpdateEventBannerVideo(pool *pgxpool.Pool, eventID, organizerID, videoURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Coerce empty string to NULL so the column is genuinely NULL when cleared.
	var val *string
	if videoURL != "" {
		val = &videoURL
	}

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET    banner_video_url = $1,
		       updated_at       = CURRENT_TIMESTAMP
		WHERE  id = $2 AND organizer_id = $3
	`, val, eventID, organizerID)
	if err != nil {
		return fmt.Errorf("failed to update banner video: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

// AppendGalleryImage adds one image to the gallery JSONB array, enforcing the
// 10-image cap atomically inside the UPDATE WHERE clause. If the array has
// already reached 10 entries the row doesn't match and RowsAffected() == 0.
func AppendGalleryImage(pool *pgxpool.Pool, eventID, organizerID, imageURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET    gallery_images = COALESCE(gallery_images, '[]'::jsonb) || to_jsonb($1::text),
		       updated_at     = CURRENT_TIMESTAMP
		WHERE  id             = $2
		AND    organizer_id   = $3
		AND    jsonb_array_length(COALESCE(gallery_images, '[]'::jsonb)) < 10
	`, imageURL, eventID, organizerID)
	if err != nil {
		return fmt.Errorf("failed to append gallery image: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		// Distinguish "event not found / wrong owner" from "gallery is full"
		// so the handler can return the correct HTTP status code.
		exists, checkErr := eventOwnedBy(pool, eventID, organizerID)
		if checkErr != nil {
			return fmt.Errorf("failed to verify event ownership: %w", checkErr)
		}
		if !exists {
			return ErrEventNotFound
		}
		return ErrGalleryFull
	}
	return nil
}

// eventOwnedBy is a helper used when an UPDATE returns 0 rows and we need to
// distinguish "not found / wrong owner" from a business-rule failure.
func eventOwnedBy(pool *pgxpool.Pool, eventID, organizerID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM events WHERE id = $1 AND organizer_id = $2)`,
		eventID, organizerID,
	).Scan(&exists)
	return exists, err
}

// GetEventImageURLs returns the cover image, banner image, banner video URL,
// and the full gallery slice for an event owned by organizerID.
//
// NOTE: the previous version of this function always returned nil for the
// gallery — the Scan was missing the gallery_images column entirely. Fixed.
func GetEventImageURLs(pool *pgxpool.Pool, eventID, organizerID string) (
	cover *string,
	bannerImage *string,
	bannerVideo *string,
	gallery []string,
	err error,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rawGallery []byte
	err = pool.QueryRow(ctx, `
		SELECT cover_image_url,
		       banner_image_url,
		       banner_video_url,
		       gallery_images
		FROM   events
		WHERE  id = $1 AND organizer_id = $2
	`, eventID, organizerID).Scan(&cover, &bannerImage, &bannerVideo, &rawGallery)
	if err != nil {
		return nil, nil, nil, nil, ErrEventNotFound
	}

	gallery = parseJSONBStrings(rawGallery)
	return cover, bannerImage, bannerVideo, gallery, nil
}

// parseJSONBStrings safely unmarshals a JSONB byte slice into a []string.
// Returns an empty non-nil slice for NULL, "null", and "[]" so callers never
// need to nil-check the result.
func parseJSONBStrings(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	s := string(raw)
	if s == "null" || s == "[]" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}
	}
	return out
}