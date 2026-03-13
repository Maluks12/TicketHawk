package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdateEventCoverImage(pool *pgxpool.Pool, eventID, organizerID, imageURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE events
		SET cover_image_url = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND organizer_id = $3
	`
	cmd, err := pool.Exec(ctx, query, imageURL, eventID, organizerID)
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

	query := `
		UPDATE events
		SET banner_image_url = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND organizer_id = $3
	`
	cmd, err := pool.Exec(ctx, query, imageURL, eventID, organizerID)
	if err != nil {
		return fmt.Errorf("failed to update banner image: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrEventNotFound
	}

	return nil
}

func AppendGalleryImage(pool *pgxpool.Pool, eventID, organizerID, imageURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE events
		SET gallery_images = COALESCE(gallery_images, '[]'::jsonb)  || to_jsonb($1::text),
		updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND organizer_id = $3
	`

	cmd, err := pool.Exec(ctx, query, imageURL, eventID, organizerID)
	if err != nil {
		return fmt.Errorf("failed to append gallery image: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

func GetEventImageURLs(pool *pgxpool.Pool, eventID, organizerID string) (cover, banner *string, gallery []string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
 
	query := `
		SELECT cover_image_url, banner_image_url
		FROM events
		WHERE id = $1 AND organizer_id = $2
	`
	err = pool.QueryRow(ctx, query, eventID,organizerID).Scan(&cover, &banner)
	if err != nil {
		return nil, nil, nil, ErrEventNotFound
	}

	return cover, banner, nil, nil
}