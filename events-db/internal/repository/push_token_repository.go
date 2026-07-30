package repository

import (
	"context"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Push token registry
//
// NOTE ON SLICE PARAMETERS: every function below that passes a slice to
// `= ANY($1)` uses a plain []string, built explicitly with make([]string, 0, n)
// and appended to. Do NOT change these to a named/defined slice type
// (`type tokenList []string`) or to a slice of a defined string type
// (`[]models.PushPlatform`). pgx v5's binary encoder resolves the OID from the
// concrete Go type, and a defined type it doesn't recognise silently encodes as
// an empty array — the query then returns zero rows with no error at all. That
// exact failure mode has already cost this codebase three production bugs
// (the TagArray.Scan one and the capacity check that crashed every booking).
// Plain []string is the type pgx recognises. Keep it plain.
// ─────────────────────────────────────────────────────────────────────────────

// UpsertPushToken registers (or re-registers) a device for a user.
//
// The conflict target is `token`, not (user_id, token), and the DO UPDATE
// re-points user_id. That is the shared-handset case: Expo issues one token per
// app INSTALL, so when a second person signs in on the same phone the app hands
// us a token we've already seen under someone else's account. Re-pointing it
// moves delivery to the current owner; inserting a second row would keep
// pushing the previous user's bookings to a phone they've signed out of.
func UpsertPushToken(
	pool *pgxpool.Pool,
	ctx context.Context,
	userID string,
	token string,
	platform models.PushPlatform,
	app models.PushApp,
	deviceName string,
) error {
	var devicePtr *string
	if deviceName != "" {
		devicePtr = &deviceName
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO push_tokens (user_id, token, platform, app, device_name, last_used_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (token) DO UPDATE
		SET user_id       = EXCLUDED.user_id,
		    platform      = EXCLUDED.platform,
		    app           = EXCLUDED.app,
		    device_name   = COALESCE(EXCLUDED.device_name, push_tokens.device_name),
		    failure_count = 0,
		    updated_at    = NOW()`,
		userID, token, string(platform), string(app), devicePtr)

	return err
}

// DeletePushToken removes a single device registration.
//
// Scoped by user_id as well as token so one account cannot unregister another
// account's device by guessing a token. Returns nil when nothing matched:
// unregistering an already-unregistered device is a no-op, not an error, since
// the app fires this on every logout regardless of whether it ever registered.
func DeletePushToken(pool *pgxpool.Pool, ctx context.Context, userID, token string) error {
	_, err := pool.Exec(ctx,
		`DELETE FROM push_tokens WHERE user_id = $1 AND token = $2`,
		userID, token)
	return err
}

// DeletePushTokensByValue removes tokens regardless of owner.
//
// Called by the push service when Expo reports DeviceNotRegistered — the app
// was uninstalled or the token rotated, so the row is dead no matter whose it
// is. There is no user to scope by at that point; the token itself is the key.
func DeletePushTokensByValue(pool *pgxpool.Pool, ctx context.Context, tokens []string) (int64, error) {
	if len(tokens) == 0 {
		return 0, nil
	}

	// Plain []string — see the slice-parameter note at the top of this file.
	list := make([]string, 0, len(tokens))
	list = append(list, tokens...)

	cmd, err := pool.Exec(ctx, `DELETE FROM push_tokens WHERE token = ANY($1)`, list)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

// DeleteAllPushTokensForUser drops every device for a user. Used by the
// "disable notifications everywhere" path and by account deletion (the FK
// cascade covers deletion too, but being explicit costs nothing).
func DeleteAllPushTokensForUser(pool *pgxpool.Pool, ctx context.Context, userID string) error {
	_, err := pool.Exec(ctx, `DELETE FROM push_tokens WHERE user_id = $1`, userID)
	return err
}

// GetPushTargets resolves a batch of user IDs to their registered devices in a
// SINGLE query.
//
// Batching matters here more than anywhere else in the pipeline: the sell-out
// worker fans one notification out to every follower of an event, which can be
// thousands of users. Resolving tokens per-user inside that loop would be
// thousands of round-trips per alert. The push service collects a batch of
// pending sends first, then calls this once for all of them.
func GetPushTargets(pool *pgxpool.Pool, ctx context.Context, userIDs []string) ([]models.UserPushTarget, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	// Plain []string — see the slice-parameter note at the top of this file.
	ids := make([]string, 0, len(userIDs))
	ids = append(ids, userIDs...)

	rows, err := pool.Query(ctx, `
		SELECT user_id, token, platform
		FROM   push_tokens
		WHERE  user_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]models.UserPushTarget, 0, len(userIDs))
	for rows.Next() {
		var t models.UserPushTarget
		var platform string
		if err := rows.Scan(&t.UserID, &t.Token, &platform); err != nil {
			return nil, err
		}
		t.Platform = models.PushPlatform(platform)
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// GetUnreadCounts returns unread notification counts for a batch of users, for
// the iOS badge number. One grouped query per batch rather than one per
// recipient — same reasoning as GetPushTargets.
//
// Users with zero unread notifications simply don't appear in the map; callers
// treat a missing key as 0.
func GetUnreadCounts(pool *pgxpool.Pool, ctx context.Context, userIDs []string) (map[string]int, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	// Plain []string — see the slice-parameter note at the top of this file.
	ids := make([]string, 0, len(userIDs))
	ids = append(ids, userIDs...)

	rows, err := pool.Query(ctx, `
		SELECT user_id, COUNT(*)
		FROM   notifications
		WHERE  user_id = ANY($1) AND is_read = false
		GROUP  BY user_id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int, len(userIDs))
	for rows.Next() {
		var uid string
		var n int
		if err := rows.Scan(&uid, &n); err != nil {
			return nil, err
		}
		counts[uid] = n
	}
	return counts, rows.Err()
}

// MarkPushTokensUsed stamps last_used_at after a successful send. Best-effort:
// a failure here is logged and swallowed, since it has no bearing on whether
// the notification was actually delivered.
func MarkPushTokensUsed(pool *pgxpool.Pool, ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}

	// Plain []string — see the slice-parameter note at the top of this file.
	list := make([]string, 0, len(tokens))
	list = append(list, tokens...)

	_, err := pool.Exec(ctx, `
		UPDATE push_tokens
		SET    last_used_at  = NOW(),
		       failure_count = 0
		WHERE  token = ANY($1)`, list)
	return err
}

// IncrementPushFailures bumps the failure counter for tokens that errored in a
// way that isn't definitively fatal (rate limits, transient Expo 5xx). Nothing
// currently acts on the counter automatically — it is a diagnostic, so a token
// that quietly rots without ever returning DeviceNotRegistered is visible in
// the table rather than invisible.
func IncrementPushFailures(pool *pgxpool.Pool, ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}

	// Plain []string — see the slice-parameter note at the top of this file.
	list := make([]string, 0, len(tokens))
	list = append(list, tokens...)

	_, err := pool.Exec(ctx, `
		UPDATE push_tokens
		SET    failure_count = failure_count + 1,
		       updated_at    = NOW()
		WHERE  token = ANY($1)`, list)
	return err
}