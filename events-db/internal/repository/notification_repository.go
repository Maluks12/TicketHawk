package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Push fan-out hook
//
// Every out-of-app notification the platform sends originates from a row in
// `notifications`, written by CreateNotification. There are six producers
// (booking worker × 2 paths, reminder worker, expiry worker, sell-out worker
// × 2 thresholds, KYC approval), each with its own call site.
//
// Rather than thread a *services.PushService through all six — which would mean
// changing every worker signature, and would make internal/repository import
// internal/services (an import cycle, since services already imports
// repository) — the push service REGISTERS itself here once at startup.
// CreateNotification then fires it after the row commits, so push delivery is
// automatic for every existing producer and for any added later.
//
// The cost of that convenience: the dispatch is invisible at the call site.
// If you ever want a notification that lands in the in-app feed but does NOT
// buzz the phone, use CreateNotificationSilent below rather than adding a flag
// here.
//
// atomic.Pointer, not a plain var: main.go starts the expiry worker before the
// push service is constructed, and that worker runs its first cycle
// immediately. Without the atomic there is a genuine (if narrow) data race
// between that first cycle writing notifications and SetPushDispatcher writing
// the var.
// ─────────────────────────────────────────────────────────────────────────────

// PushDispatchFunc is implemented by services.PushService.Dispatch. It MUST NOT
// block — the contract is that it enqueues onto a buffered channel and returns
// immediately, dropping (with a log line) if the queue is saturated. Workers
// call CreateNotification in tight fan-out loops; a dispatcher that blocked on
// an HTTP call would stall the loop for thousands of followers.
type PushDispatchFunc func(
	userID string,
	notifType models.NotificationType,
	title string,
	body string,
	data json.RawMessage,
)

var pushDispatcher atomic.Pointer[PushDispatchFunc]

// SetPushDispatcher wires the push service in. Called once from main.go.
// Passing nil disables push (the in-app feed keeps working untouched), which
// is what happens when PUSH_ENABLED=false.
func SetPushDispatcher(fn PushDispatchFunc) {
	if fn == nil {
		pushDispatcher.Store(nil)
		return
	}
	pushDispatcher.Store(&fn)
}

func CreateNotification(
	pool      *pgxpool.Pool,
	ctx       context.Context,
	userID    string,
	notifType models.NotificationType,
	title     string,
	body      string,
	data      any,
) error {
	dataBytes, err := createNotificationRow(pool, ctx, userID, notifType, title, body, data)
	if err != nil {
		return err
	}

	// Fire the push AFTER the insert succeeds, never before. If the row write
	// fails the user must not get a phone buzz for a notification that isn't
	// in their feed — tapping it would open an empty inbox.
	if p := pushDispatcher.Load(); p != nil {
		(*p)(userID, notifType, title, body, dataBytes)
	}
	return nil
}

// CreateNotificationSilent writes the feed row WITHOUT sending a push.
//
// Use this for anything that should be discoverable when the user next opens
// the app but isn't worth interrupting them for. Nothing calls it today; it
// exists so the "add a notification but don't buzz" case has an obvious answer
// that isn't "add a bool to CreateNotification and update six call sites".
func CreateNotificationSilent(
	pool      *pgxpool.Pool,
	ctx       context.Context,
	userID    string,
	notifType models.NotificationType,
	title     string,
	body      string,
	data      any,
) error {
	_, err := createNotificationRow(pool, ctx, userID, notifType, title, body, data)
	return err
}

func createNotificationRow(
	pool      *pgxpool.Pool,
	ctx       context.Context,
	userID    string,
	notifType models.NotificationType,
	title     string,
	body      string,
	data      any,
) (json.RawMessage, error) {
	var dataBytes []byte
	if data != nil {
		var err error
		dataBytes, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("notification: marshal data: %w", err)
		}
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO notifications (user_id, type, title, body, data)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, notifType, title, body, dataBytes)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(dataBytes), nil
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