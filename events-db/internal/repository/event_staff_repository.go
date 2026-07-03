package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Event staffing ────────────────────────────────────────────────────────────
//
// Membership model behind door scanning. The event OWNER (events.organizer_id,
// a users.id) is implicit and never stored in event_staff; authorization checks
// OR the two together. Staff rows are soft-revoked so "who could scan when"
// stays answerable.

// Sentinel errors the handlers translate into precise HTTP responses.
var (
	//ErrUserNotFound   = errors.New("user not found")
	ErrCodeInvalid    = errors.New("invalid or expired code") // uniform on purpose (no oracle)
	ErrNotAuthorized  = errors.New("not authorized")
	//ErrEventNotFound  = errors.New("event not found")
	ErrCannotSelfDrop = errors.New("owner cannot be revoked")
)

// StaffMember is one row of an event's staff list, joined to the user profile.
type StaffMember struct {
	UserID    string     `json:"user_id"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	AddedBy   string     `json:"added_by"`
	ViaCode   bool       `json:"via_code"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// StaffCode is one join code with its live usage.
type StaffCode struct {
	ID         string     `json:"id"`
	Code       string     `json:"code"` // display form: XXXX-XXXX
	GrantsRole string     `json:"grants_role"`
	MaxUses    int        `json:"max_uses"`
	UseCount   int        `json:"use_count"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ScannableEvent is one event the caller may scan, for the scan-screen picker.
type ScannableEvent struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	VenueName *string   `json:"venue_name,omitempty"`
	City      *string   `json:"city,omitempty"`
	Role      string    `json:"role"` // "owner", "manager", or "scanner"
}

// CanManageEventStaff reports whether userID may manage staff/codes for the
// event: the owner, a platform admin, or an active manager-role staffer.
func CanManageEventStaff(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, userID, globalRole string,
) (bool, error) {
	if globalRole == "admin" {
		// Still confirm the event exists so admins get a clean 404.
		var one int
		if err := pool.QueryRow(ctx, `SELECT 1 FROM events WHERE id = $1`, eventID).Scan(&one); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, ErrEventNotFound
			}
			return false, err
		}
		return true, nil
	}

	var allowed bool
	err := pool.QueryRow(ctx, `
		SELECT
			e.organizer_id = $2
			OR EXISTS (
				SELECT 1 FROM event_staff es
				WHERE es.event_id = e.id
				  AND es.user_id  = $2
				  AND es.status   = 'active'
				  AND es.role     = 'manager'
			)
		FROM events e
		WHERE e.id = $1
	`, eventID, userID).Scan(&allowed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrEventNotFound
		}
		return false, err
	}
	return allowed, nil
}

// ListEventStaff returns the event's staff (active first, then revoked),
// joined to user profiles.
func ListEventStaff(ctx context.Context, pool *pgxpool.Pool, eventID string) ([]StaffMember, error) {
	rows, err := pool.Query(ctx, `
		SELECT es.user_id,
		       COALESCE(u.first_name, ''), COALESCE(u.last_name, ''), u.email,
		       es.role::text, es.status::text,
		       COALESCE(a.email, ''),
		       es.joined_via IS NOT NULL,
		       es.created_at, es.revoked_at
		FROM event_staff es
		JOIN users u      ON u.id = es.user_id
		LEFT JOIN users a ON a.id = es.added_by
		WHERE es.event_id = $1
		ORDER BY (es.status = 'active') DESC, es.created_at ASC
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StaffMember{}
	for rows.Next() {
		var m StaffMember
		if err := rows.Scan(
			&m.UserID, &m.FirstName, &m.LastName, &m.Email,
			&m.Role, &m.Status, &m.AddedBy, &m.ViaCode,
			&m.CreatedAt, &m.RevokedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddStaffByEmail grants (or re-activates) staff access for the account with
// the given email. Adding the event owner is a no-op error: their access is
// already implicit and duplicating it would create two sources of truth.
func AddStaffByEmail(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, email, role, addedBy string,
) (*StaffMember, error) {
	var (
		userID    string
		firstName string
		lastName  string
	)
	err := pool.QueryRow(ctx, `
		SELECT id, COALESCE(first_name, ''), COALESCE(last_name, '')
		FROM users WHERE lower(email) = lower($1)
	`, strings.TrimSpace(email)).Scan(&userID, &firstName, &lastName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	var ownerID string
	if err := pool.QueryRow(ctx, `SELECT organizer_id FROM events WHERE id = $1`, eventID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	if ownerID == userID {
		return nil, fmt.Errorf("the event owner already has full access")
	}

	// Upsert: re-adding a revoked staffer reactivates the same row.
	_, err = pool.Exec(ctx, `
		INSERT INTO event_staff (event_id, user_id, role, status, added_by)
		VALUES ($1, $2, $3, 'active', $4)
		ON CONFLICT (event_id, user_id) DO UPDATE
		SET role = EXCLUDED.role, status = 'active',
		    added_by = EXCLUDED.added_by,
		    revoked_at = NULL, revoked_by = NULL
	`, eventID, userID, role, addedBy)
	if err != nil {
		return nil, err
	}

	return &StaffMember{
		UserID: userID, FirstName: firstName, LastName: lastName,
		Email: email, Role: role, Status: "active", CreatedAt: time.Now(),
	}, nil
}

// RevokeStaff soft-revokes a staffer's access. Returns ErrUserNotFound when
// there is no active row to revoke.
func RevokeStaff(ctx context.Context, pool *pgxpool.Pool, eventID, userID, revokedBy string) error {
	tag, err := pool.Exec(ctx, `
		UPDATE event_staff
		SET status = 'revoked', revoked_at = now(), revoked_by = $3
		WHERE event_id = $1 AND user_id = $2 AND status = 'active'
	`, eventID, userID, revokedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ── Join codes ────────────────────────────────────────────────────────────────

// crockford is base32 without the ambiguous I, L, O, U — codes stay readable
// when read out loud at a noisy gate.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func generateStaffCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i, v := range b {
		out[i] = crockford[int(v)%len(crockford)]
	}
	return string(out), nil
}

// NormalizeStaffCode strips dashes/spaces and uppercases, so 'scan-7kq4 m2xr'
// and '7KQ4M2XR' redeem identically.
func NormalizeStaffCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.NewReplacer("-", "", " ", "", "SCAN", "").Replace(code)
	return code
}

// formatStaffCode renders the stored 8-char code as the display form XXXX-XXXX.
func formatStaffCode(code string) string {
	if len(code) == 8 {
		return code[:4] + "-" + code[4:]
	}
	return code
}

// CreateStaffCode mints a join code for the event. expiresAt of zero defaults
// to the event's admission-window end (end_time + 2h) so forgotten codes die
// with the event.
func CreateStaffCode(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, grantsRole string,
	maxUses int,
	expiresAt time.Time,
	createdBy string,
) (*StaffCode, error) {
	if maxUses <= 0 {
		maxUses = 10
	}
	if expiresAt.IsZero() {
		var endTime time.Time
		if err := pool.QueryRow(ctx, `SELECT end_time FROM events WHERE id = $1`, eventID).Scan(&endTime); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrEventNotFound
			}
			return nil, err
		}
		expiresAt = endTime.Add(2 * time.Hour)
	}

	// Retry on the (astronomically unlikely) unique collision.
	for attempt := 0; attempt < 3; attempt++ {
		code, err := generateStaffCode()
		if err != nil {
			return nil, err
		}
		var id string
		var createdAt time.Time
		err = pool.QueryRow(ctx, `
			INSERT INTO event_staff_codes (event_id, code, grants_role, max_uses, expires_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at
		`, eventID, code, grantsRole, maxUses, expiresAt, createdBy).Scan(&id, &createdAt)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				continue
			}
			return nil, err
		}
		return &StaffCode{
			ID: id, Code: formatStaffCode(code), GrantsRole: grantsRole,
			MaxUses: maxUses, UseCount: 0, ExpiresAt: expiresAt, CreatedAt: createdAt,
		}, nil
	}
	return nil, fmt.Errorf("could not generate a unique code")
}

// ListStaffCodes returns the event's codes, newest first.
func ListStaffCodes(ctx context.Context, pool *pgxpool.Pool, eventID string) ([]StaffCode, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, code, grants_role::text, max_uses, use_count, expires_at, revoked_at, created_at
		FROM event_staff_codes
		WHERE event_id = $1
		ORDER BY created_at DESC
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StaffCode{}
	for rows.Next() {
		var sc StaffCode
		if err := rows.Scan(&sc.ID, &sc.Code, &sc.GrantsRole, &sc.MaxUses, &sc.UseCount,
			&sc.ExpiresAt, &sc.RevokedAt, &sc.CreatedAt); err != nil {
			return nil, err
		}
		sc.Code = formatStaffCode(sc.Code)
		out = append(out, sc)
	}
	return out, rows.Err()
}

// RevokeStaffCode revokes a code. People who already joined through it keep
// their membership (individually revocable from the staff list).
func RevokeStaffCode(ctx context.Context, pool *pgxpool.Pool, eventID, codeID string) error {
	tag, err := pool.Exec(ctx, `
		UPDATE event_staff_codes
		SET revoked_at = now()
		WHERE id = $1 AND event_id = $2 AND revoked_at IS NULL
	`, codeID, eventID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCodeInvalid
	}
	return nil
}

// RedeemStaffCode atomically consumes one use of a code and grants (or
// re-activates) the redeeming user's membership. The use-count increment and
// the membership upsert commit together, so concurrent redeems can never
// exceed max_uses. All failure modes return the same ErrCodeInvalid — a wrong
// code learns nothing about whether it once existed.
func RedeemStaffCode(
	ctx context.Context,
	pool *pgxpool.Pool,
	code, userID string,
) (*ScannableEvent, error) {
	norm := NormalizeStaffCode(code)
	if len(norm) != 8 {
		return nil, ErrCodeInvalid
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		codeID     string
		eventID    string
		grantsRole string
	)
	err = tx.QueryRow(ctx, `
		UPDATE event_staff_codes
		SET use_count = use_count + 1
		WHERE code = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
		  AND use_count < max_uses
		RETURNING id, event_id, grants_role::text
	`, norm).Scan(&codeID, &eventID, &grantsRole)
	if err != nil {
		return nil, ErrCodeInvalid // not found / expired / revoked / used up — uniform
	}

	// Owner redeeming their own event's code: harmless, but don't create a row.
	var ownerID string
	if err := tx.QueryRow(ctx, `SELECT organizer_id FROM events WHERE id = $1`, eventID).Scan(&ownerID); err != nil {
		return nil, err
	}
	if ownerID != userID {
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_staff (event_id, user_id, role, status, added_by, joined_via)
			VALUES ($1, $2, $3, 'active', $2, $4)
			ON CONFLICT (event_id, user_id) DO UPDATE
			SET role = EXCLUDED.role, status = 'active',
			    revoked_at = NULL, revoked_by = NULL,
			    joined_via = EXCLUDED.joined_via
		`, eventID, userID, grantsRole, codeID); err != nil {
			return nil, err
		}
	}

	var ev ScannableEvent
	if err := tx.QueryRow(ctx, `
		SELECT id, title, start_time, end_time, venue_name, city
		FROM events WHERE id = $1
	`, eventID).Scan(&ev.ID, &ev.Title, &ev.StartTime, &ev.EndTime, &ev.VenueName, &ev.City); err != nil {
		return nil, err
	}
	ev.Role = grantsRole
	if ownerID == userID {
		ev.Role = "owner"
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ev, nil
}

// ListScannableEvents returns the events the user may scan: owned ∪ active
// staff memberships, upcoming-or-recent first. Admins also get this list (their
// blanket scan right doesn't need enumerating every event on the platform).
func ListScannableEvents(ctx context.Context, pool *pgxpool.Pool, userID string) ([]ScannableEvent, error) {
	rows, err := pool.Query(ctx, `
		SELECT e.id, e.title, e.start_time, e.end_time, e.venue_name, e.city, 'owner' AS role
		FROM events e
		WHERE e.organizer_id = $1
		UNION ALL
		SELECT e.id, e.title, e.start_time, e.end_time, e.venue_name, e.city, es.role::text
		FROM event_staff es
		JOIN events e ON e.id = es.event_id
		WHERE es.user_id = $1 AND es.status = 'active'
		ORDER BY end_time DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ScannableEvent{}
	for rows.Next() {
		var ev ScannableEvent
		if err := rows.Scan(&ev.ID, &ev.Title, &ev.StartTime, &ev.EndTime, &ev.VenueName, &ev.City, &ev.Role); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}