package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateNotification(
	pool      *pgxpool.Pool,
	ctx       context.Context,
	userID    string,
	notifType models.NotificationType,
	title     string,
	body      string,
	data      any,
) error {
	var dataBytes []byte
	if data != nil {
		var err error
		dataBytes, err = json.Marshal(data)
		if err != nil {
			return fmt.Errorf("notification: marshal data: %w", err)
		}
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO notifications (user_id, type, title, body, data)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, notifType, title, body, dataBytes)
	return err
}

func GetNotifications(
	pool   *pgxpool.Pool,
	userID string,
	limit  int,
	cursor *utils.Cursor,
) ([]models.Notification, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id, user_id, type, title, body, data, is_read, created_at
		FROM   notifications
		WHERE  user_id = $1
		ORDER  BY created_at DESC
		LIMIT  $2`, userID, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	notifs := []models.Notification{} //might need to change this
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Data, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, nil, err
		}
		notifs = append(notifs, n)
	}

	var nextCursor *utils.Cursor
	if len(notifs) > limit {
		last := notifs[limit-1]
		notifs = notifs[:limit]
		nextCursor = &utils.Cursor{ID: last.ID, CreatedAt: last.CreatedAt}
	}
	return notifs, nextCursor, nil
}

func GetUnreadCount(pool *pgxpool.Pool, ctx context.Context, userID string) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID,
	).Scan(&count)
	return count, err
}

func MarkAsRead(pool *pgxpool.Pool, ctx context.Context, userID, notifID string) error {
	cmd, err := pool.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2`,
		notifID, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("notification not found")
	}
	return nil
}

func MarkAllRead(pool *pgxpool.Pool, ctx context.Context, userID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE user_id = $1 AND is_read = false`, userID)
	return err
}