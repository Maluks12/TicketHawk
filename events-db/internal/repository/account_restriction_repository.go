package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Account restrictions (migration 000094) — MASTER ADMIN ONLY ─────────────
//
// A restriction row is written once and changed only to lift it. Lifting — by
// a master admin, or because a newer restriction on the same thing replaced it
// — sets lifted_at; a row whose ends_at has passed simply stops being in
// force. The table is its own history.
//
// Every write happens in ONE transaction with its staff_audit_log row and, for
// a lockout, with the revocation of the account's refresh tokens. An account
// is never locked out without its sessions ending, and never restricted
// without the record: if any part fails, nothing is written.
//
// Enforcement lives elsewhere — internal/accountguard (in memory, on every
// request), FetchPayableRows and splitForBooking (payouts). The handler reloads
// the registry after every successful write here.

var (
	ErrRestrictionAccountNotFound = errors.New("account not found")
	ErrRestrictionSelf            = errors.New("you can't restrict your own account")
	ErrRestrictionMasterAdmin     = errors.New("master admin accounts can't be restricted")
	ErrRestrictionNotFound        = errors.New("restriction not found")
	ErrRestrictionNotInForce      = errors.New("that restriction has already ended or been lifted")
	ErrRestrictionAction          = errors.New("action must be disable, ban or block")
	ErrRestrictionDays            = fmt.Errorf("days must be a whole number from 1 to %d", models.MaxRestrictionDays)
	ErrRestrictionNoFeatures      = errors.New("choose at least one feature to block")
	ErrRestrictionFeature         = errors.New("unknown feature")
	ErrRestrictionReason          = fmt.Errorf("give a reason of %d to %d characters", models.MinRestrictionReasonLen, models.MaxRestrictionReasonLen)
	ErrRestrictionNoteTooLong     = fmt.Errorf("the note must be %d characters or fewer", models.MaxRestrictionReasonLen)
)

// Actions the console sends.
const (
	RestrictActionDisable = "disable"
	RestrictActionBan     = "ban"
	RestrictActionBlock   = "block"
)

// RestrictionInput is one master-admin action against an account.
type RestrictionInput struct {
	Action   string
	Days     *int // disable: required. block: nil or 0 = indefinite. ban: ignored.
	Features []models.RestrictionKind
	Reason   string
}

// restrictionPlan is a validated RestrictionInput.
type restrictionPlan struct {
	kinds    []models.RestrictionKind // rows to write
	replaces []models.RestrictionKind // kinds whose rows in force are lifted first
	days     *int                     // nil = indefinite
	reason   string
}

// planRestriction validates an input. Pure, so the rules are testable without
// a database; the CHECK constraints in 000094 are the backstop.
func planRestriction(in RestrictionInput) (restrictionPlan, error) {
	reason := strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(reason); n < models.MinRestrictionReasonLen || n > models.MaxRestrictionReasonLen {
		return restrictionPlan{}, ErrRestrictionReason
	}
	p := restrictionPlan{reason: reason}

	switch in.Action {
	case RestrictActionDisable:
		if in.Days == nil || *in.Days < 1 || *in.Days > models.MaxRestrictionDays {
			return restrictionPlan{}, ErrRestrictionDays
		}
		d := *in.Days
		p.days = &d
		p.kinds = []models.RestrictionKind{models.RestrictDisable}
		// A new lockout replaces whichever lockout is in force: disabling a
		// banned account turns the ban into the disable, and banning a
		// disabled account makes the lockout indefinite.
		p.replaces = []models.RestrictionKind{models.RestrictDisable, models.RestrictBan}

	case RestrictActionBan:
		p.kinds = []models.RestrictionKind{models.RestrictBan}
		p.replaces = []models.RestrictionKind{models.RestrictDisable, models.RestrictBan}

	case RestrictActionBlock:
		if len(in.Features) == 0 {
			return restrictionPlan{}, ErrRestrictionNoFeatures
		}
		if in.Days != nil && *in.Days != 0 {
			if *in.Days < 1 || *in.Days > models.MaxRestrictionDays {
				return restrictionPlan{}, ErrRestrictionDays
			}
			d := *in.Days
			p.days = &d
		}
		seen := map[models.RestrictionKind]bool{}
		for _, f := range in.Features {
			if !f.IsFeature() {
				return restrictionPlan{}, ErrRestrictionFeature
			}
			if !seen[f] {
				seen[f] = true
				p.kinds = append(p.kinds, f)
			}
		}
		// Each feature replaces its own block in force and nothing else, so
		// blocking payouts for 7 days leaves an indefinite ticket block alone.
		p.replaces = p.kinds

	default:
		return restrictionPlan{}, ErrRestrictionAction
	}
	return p, nil
}

func kindsToStrings(kinds []models.RestrictionKind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// ApplyResult is what one action did.
type ApplyResult struct {
	RestrictionIDs []string `json:"restriction_ids"`
	ReplacedIDs    []string `json:"replaced_ids"`
	SessionsEnded  int64    `json:"sessions_ended"`
}

// ApplyAccountRestriction applies one master-admin action to an account:
// replaces whatever it supersedes, writes the new rows, ends every session if
// it is a lockout, and writes the audit row — all in one transaction.
func ApplyAccountRestriction(ctx context.Context, pool *pgxpool.Pool, userID string, in RestrictionInput, actor AuditActor) (ApplyResult, error) {
	res := ApplyResult{RestrictionIDs: []string{}, ReplacedIDs: []string{}}

	p, err := planRestriction(in)
	if err != nil {
		return res, err
	}
	// A master admin locking themselves out would need psql to undo.
	if userID == actor.UserID {
		return res, ErrRestrictionSelf
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Locks the account row, so two master admins acting on the same account
	// at the same moment are applied one after the other rather than each
	// replacing a restriction the other has just written.
	var isMaster, deleted bool
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(is_master_admin, FALSE), deleted_at IS NOT NULL
		FROM users
		WHERE id = $1
		FOR UPDATE
	`, userID).Scan(&isMaster, &deleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return res, ErrRestrictionAccountNotFound
		}
		return res, fmt.Errorf("failed to load account: %w", err)
	}
	if deleted {
		return res, ErrRestrictionAccountNotFound
	}
	if isMaster {
		return res, ErrRestrictionMasterAdmin
	}

	rows, err := tx.Query(ctx, `
		UPDATE account_restrictions
		SET lifted_at   = now(),
		    lifted_by   = $2,
		    lift_reason = 'Replaced by a newer restriction'
		WHERE user_id = $1
		  AND kind = ANY($3::text[])
		  AND lifted_at IS NULL
		  AND (ends_at IS NULL OR ends_at > now())
		RETURNING id::text
	`, userID, actor.UserID, kindsToStrings(p.replaces))
	if err != nil {
		return res, fmt.Errorf("failed to replace restrictions in force: %w", err)
	}
	replaced, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return res, fmt.Errorf("failed to replace restrictions in force: %w", err)
	}
	res.ReplacedIDs = append(res.ReplacedIDs, replaced...)

	for _, k := range p.kinds {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO account_restrictions (user_id, kind, reason, created_by, ends_at)
			VALUES ($1, $2, $3, $4,
			        CASE WHEN $5::int IS NULL THEN NULL
			             ELSE now() + make_interval(days => $5::int) END)
			RETURNING id::text
		`, userID, string(k), p.reason, actor.UserID, p.days).Scan(&id)
		if err != nil {
			return res, fmt.Errorf("failed to write restriction: %w", err)
		}
		res.RestrictionIDs = append(res.RestrictionIDs, id)
	}

	// A lockout ends every session in the same transaction. The in-memory
	// registry refuses the account's access tokens; this makes sure no refresh
	// token can mint a new one, even if the registry were somehow behind.
	if p.kinds[0].IsLockout() {
		tag, err := tx.Exec(ctx, `
			UPDATE refresh_tokens
			SET revoked        = TRUE,
			    revoked_at     = NOW(),
			    revoked_reason = $2
			WHERE user_id = $1
			  AND revoked = FALSE
		`, userID, RevokeReasonRestricted)
		if err != nil {
			return res, fmt.Errorf("failed to end sessions: %w", err)
		}
		res.SessionsEnded = tag.RowsAffected()
	}

	detail := map[string]any{
		"action":          in.Action,
		"kinds":           kindsToStrings(p.kinds),
		"days":            p.days,
		"reason":          p.reason,
		"restriction_ids": res.RestrictionIDs,
		"replaced_ids":    res.ReplacedIDs,
		"sessions_ended":  res.SessionsEnded,
	}
	if err = writeRestrictionAudit(ctx, tx, actor, "user.restrict", userID, detail); err != nil {
		return res, err
	}

	if err = tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("failed to commit restriction: %w", err)
	}
	return res, nil
}

// LiftAccountRestriction ends one restriction in force, now. Returns the
// account it belonged to. Sessions a lockout ended stay ended: the account
// holder simply signs in again.
func LiftAccountRestriction(ctx context.Context, pool *pgxpool.Pool, restrictionID, note string, actor AuditActor) (string, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > models.MaxRestrictionReasonLen {
		return "", ErrRestrictionNoteTooLong
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID, kind string
	var inForce bool
	err = tx.QueryRow(ctx, `
		SELECT user_id::text, kind,
		       (lifted_at IS NULL AND (ends_at IS NULL OR ends_at > now()))
		FROM account_restrictions
		WHERE id = $1
		FOR UPDATE
	`, restrictionID).Scan(&userID, &kind, &inForce)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrRestrictionNotFound
		}
		return "", fmt.Errorf("failed to load restriction: %w", err)
	}
	if !inForce {
		return "", ErrRestrictionNotInForce
	}

	var liftReason *string
	if note != "" {
		liftReason = &note
	}
	if _, err = tx.Exec(ctx, `
		UPDATE account_restrictions
		SET lifted_at = now(), lifted_by = $2, lift_reason = $3
		WHERE id = $1
	`, restrictionID, actor.UserID, liftReason); err != nil {
		return "", fmt.Errorf("failed to lift restriction: %w", err)
	}

	detail := map[string]any{
		"restriction_id": restrictionID,
		"kind":           kind,
		"note":           liftReason,
	}
	if err = writeRestrictionAudit(ctx, tx, actor, "user.restriction_lift", userID, detail); err != nil {
		return "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit lift: %w", err)
	}
	return userID, nil
}

// writeRestrictionAudit records a restriction change inside the caller's
// transaction, so the change and its record commit or roll back together.
func writeRestrictionAudit(ctx context.Context, tx pgx.Tx, actor AuditActor, action, userID string, detail map[string]any) error {
	var ip *string
	if actor.IP != "" {
		ip = &actor.IP
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO staff_audit_log
			(actor_id, actor_email, actor_role, action, target_type, target_id,
			 allowed, detail, ip, user_agent)
		VALUES ($1, $2, (SELECT staff_role FROM users WHERE id = $1),
		        $3, 'user', $4::uuid,
		        TRUE, $5, $6::inet, $7)
	`, actor.UserID, actor.Email, action, userID, detail, ip, actor.UserAgent)
	if err != nil {
		return fmt.Errorf("failed to write audit log: %w", err)
	}
	return nil
}

// ── Console reads ────────────────────────────────────────────────────────────

// RestrictionRecord is one restriction as the console shows it, in force or
// past.
type RestrictionRecord struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Kind           string     `json:"kind"`
	Label          string     `json:"label"`
	Reason         string     `json:"reason"`
	CreatedAt      time.Time  `json:"created_at"`
	CreatedByEmail *string    `json:"created_by_email"`
	EndsAt         *time.Time `json:"ends_at"`
	LiftedAt       *time.Time `json:"lifted_at"`
	LiftedByEmail  *string    `json:"lifted_by_email"`
	LiftReason     *string    `json:"lift_reason"`
	// "active", "expired" or "lifted", and the whole days left (rounded up,
	// null when indefinite or over) — both decided by the database clock so
	// the console never compares dates against the viewer's own clock.
	State         string `json:"state"`
	DaysRemaining *int   `json:"days_remaining"`
}

const restrictionRecordSelect = `
	SELECT ar.id::text, ar.user_id::text, ar.kind, ar.reason, ar.created_at, cb.email,
	       ar.ends_at, ar.lifted_at, lb.email, ar.lift_reason,
	       CASE WHEN ar.lifted_at IS NOT NULL THEN 'lifted'
	            WHEN ar.ends_at IS NOT NULL AND ar.ends_at <= now() THEN 'expired'
	            ELSE 'active' END,
	       CASE WHEN ar.lifted_at IS NULL AND ar.ends_at > now()
	            THEN CEIL(EXTRACT(EPOCH FROM (ar.ends_at - now())) / 86400)::int
	       END
	FROM account_restrictions ar
	LEFT JOIN users cb ON cb.id = ar.created_by
	LEFT JOIN users lb ON lb.id = ar.lifted_by
`

func scanRestrictionRecords(rows pgx.Rows) ([]RestrictionRecord, error) {
	defer rows.Close()
	out := []RestrictionRecord{}
	for rows.Next() {
		var r RestrictionRecord
		if err := rows.Scan(&r.ID, &r.UserID, &r.Kind, &r.Reason, &r.CreatedAt, &r.CreatedByEmail,
			&r.EndsAt, &r.LiftedAt, &r.LiftedByEmail, &r.LiftReason, &r.State, &r.DaysRemaining); err != nil {
			return nil, fmt.Errorf("failed to scan restriction: %w", err)
		}
		r.Label = models.RestrictionKind(r.Kind).Label()
		out = append(out, r)
	}
	return out, rows.Err()
}

// RestrictableAccount is one account as the console's picker and detail panel
// see it.
type RestrictableAccount struct {
	UserID        string     `json:"user_id"`
	DisplayName   string     `json:"display_name"`
	Email         string     `json:"email"`
	Role          string     `json:"role"`
	StaffRole     *string    `json:"staff_role"`
	IsMasterAdmin bool       `json:"is_master_admin"`
	BusinessName  *string    `json:"business_name"`
	CreatedAt     *time.Time `json:"created_at"`
	// The most severe restriction in force: "banned", "disabled", "limited"
	// or "active".
	State  string              `json:"state"`
	Active []RestrictionRecord `json:"active"`
}

const restrictableAccountSelect = `
	SELECT u.id::text,
	       COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.first_name, u.last_name)), ''), ''),
	       u.email, u.role::text, u.staff_role::text,
	       COALESCE(u.is_master_admin, FALSE), o.business_name, u.created_at
	FROM users u
	LEFT JOIN organizers o ON o.user_id = u.id
`

func scanRestrictableAccounts(rows pgx.Rows) ([]RestrictableAccount, error) {
	defer rows.Close()
	out := []RestrictableAccount{}
	for rows.Next() {
		var a RestrictableAccount
		if err := rows.Scan(&a.UserID, &a.DisplayName, &a.Email, &a.Role, &a.StaffRole,
			&a.IsMasterAdmin, &a.BusinessName, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		a.State = "active"
		a.Active = []RestrictionRecord{}
		out = append(out, a)
	}
	return out, rows.Err()
}

func accountState(active []RestrictionRecord) string {
	state := "active"
	for _, r := range active {
		switch models.RestrictionKind(r.Kind) {
		case models.RestrictBan:
			return "banned"
		case models.RestrictDisable:
			state = "disabled"
		default:
			if state == "active" {
				state = "limited"
			}
		}
	}
	return state
}

// attachActiveRestrictions fills Active and State for each account in one
// query.
func attachActiveRestrictions(ctx context.Context, pool *pgxpool.Pool, accounts []RestrictableAccount) error {
	if len(accounts) == 0 {
		return nil
	}
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.UserID
	}
	// text[] cast to uuid[] in SQL rather than relying on the driver to encode
	// a []string as uuid[]; the cast happens on the parameter, so the index on
	// user_id is still used.
	rows, err := pool.Query(ctx, restrictionRecordSelect+`
		WHERE ar.user_id = ANY($1::text[]::uuid[])
		  AND ar.lifted_at IS NULL
		  AND (ar.ends_at IS NULL OR ar.ends_at > now())
		ORDER BY ar.created_at DESC
	`, ids)
	if err != nil {
		return fmt.Errorf("failed to load restrictions in force: %w", err)
	}
	recs, err := scanRestrictionRecords(rows)
	if err != nil {
		return err
	}
	byUser := map[string][]RestrictionRecord{}
	for _, r := range recs {
		byUser[r.UserID] = append(byUser[r.UserID], r)
	}
	for i := range accounts {
		if active := byUser[accounts[i].UserID]; len(active) > 0 {
			accounts[i].Active = active
		}
		accounts[i].State = accountState(accounts[i].Active)
	}
	return nil
}

// SearchRestrictableAccounts backs the console's picker. An empty query lists
// every account with a restriction in force, most recently restricted first;
// otherwise it searches all live accounts by name, email or business name.
func SearchRestrictableAccounts(ctx context.Context, pool *pgxpool.Pool, q string, limit int) ([]RestrictableAccount, error) {
	rows, err := pool.Query(ctx, restrictableAccountSelect+`
		WHERE u.deleted_at IS NULL
		  AND (
		        ($1::text = '' AND EXISTS (
		            SELECT 1 FROM active_account_restrictions ar WHERE ar.user_id = u.id))
		     OR ($1::text <> '' AND (
		            u.email ILIKE '%' || $1 || '%'
		         OR TRIM(CONCAT_WS(' ', u.first_name, u.last_name)) ILIKE '%' || $1 || '%'
		         OR o.business_name ILIKE '%' || $1 || '%'))
		  )
		ORDER BY (SELECT max(ar.created_at) FROM active_account_restrictions ar
		          WHERE ar.user_id = u.id) DESC NULLS LAST,
		         u.created_at DESC NULLS LAST
		LIMIT $2
	`, q, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search accounts: %w", err)
	}
	accounts, err := scanRestrictableAccounts(rows)
	if err != nil {
		return nil, err
	}
	if err := attachActiveRestrictions(ctx, pool, accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

// GetRestrictableAccount returns one live account with its restrictions in
// force, and its whole history newest first — in force, ended and lifted.
func GetRestrictableAccount(ctx context.Context, pool *pgxpool.Pool, userID string) (*RestrictableAccount, []RestrictionRecord, error) {
	rows, err := pool.Query(ctx, restrictableAccountSelect+`
		WHERE u.id = $1
		  AND u.deleted_at IS NULL
	`, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load account: %w", err)
	}
	accounts, err := scanRestrictableAccounts(rows)
	if err != nil {
		return nil, nil, err
	}
	if len(accounts) == 0 {
		return nil, nil, ErrRestrictionAccountNotFound
	}
	if err := attachActiveRestrictions(ctx, pool, accounts); err != nil {
		return nil, nil, err
	}

	hrows, err := pool.Query(ctx, restrictionRecordSelect+`
		WHERE ar.user_id = $1
		ORDER BY ar.created_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load restriction history: %w", err)
	}
	history, err := scanRestrictionRecords(hrows)
	if err != nil {
		return nil, nil, err
	}
	return &accounts[0], history, nil
}