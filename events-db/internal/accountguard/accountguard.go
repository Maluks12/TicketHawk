// Package accountguard enforces master-admin account restrictions
// (migration 000094): a disable for a set number of days, an indefinite ban,
// and blocks on single features.
//
// # WHY AN IN-MEMORY SNAPSHOT
//
// The lockout check sits in AuthMiddleware and OptionalAuth, so it runs on
// every request that carries a token — including the rotating-QR polls every
// ticket holder's phone makes at the door. A database round trip per request,
// for a list that is almost always empty, would be the most expensive way to
// answer "no".
//
// The restrictions in force are a moderation list, not a user table, so all of
// them are held in memory and swapped atomically:
//
//   - on Start, synchronously, before the router serves anything
//   - every refresh interval, to pick up changes made elsewhere (psql, or a
//     second API instance if there is ever more than one)
//   - straight after the console writes one, via Reload, so a ban applies to
//     the next request rather than the next tick
//
// Expiry needs no reload. Every entry carries its end time and every check
// compares against the clock, so a seven-day disable lifts itself at the exact
// second it ends.
//
// # FAILURE MODE
//
// If a refresh fails, the previous snapshot stays in force: restrictions that
// were known keep holding. A registry that was never started (unit tests,
// one-off tools) restricts nobody.
package accountguard

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Codes clients branch on. The `error` string beside them is the popup text
// itself, so an app build that predates the codes still shows the right words.
const (
	CodeBanned          = "account_banned"
	CodeDisabled        = "account_disabled"
	CodeLimited         = "account_limited"
	CodePurchaseBlocked = "purchase_blocked"
)

// SupportEmail is where a locked-out account holder is pointed. Same address
// the account deletion flow gives out.
const SupportEmail = "support@ticket-hawk.com"

// Entry is one restriction in force.
type Entry struct {
	ID     string
	Kind   models.RestrictionKind
	EndsAt *time.Time // nil = indefinite
}

func (e Entry) inForce(now time.Time) bool {
	return e.EndsAt == nil || now.Before(*e.EndsAt)
}

// Lock is an account-level restriction in force: a ban or a disable.
type Lock struct {
	Kind   models.RestrictionKind
	EndsAt *time.Time // nil for a ban
}

type snapshot struct {
	byUser map[string][]Entry
	// Normalised email → user id, only for accounts that may not buy tickets.
	// Guest checkout has an email and no user id, so this is the only way it
	// can tell that the buyer is an account that has been stopped.
	byEmail map[string]string
}

func emptySnapshot() *snapshot {
	return &snapshot{byUser: map[string][]Entry{}, byEmail: map[string]string{}}
}

type registry struct {
	pool     *pgxpool.Pool
	snap     atomic.Pointer[snapshot]
	reloadMu sync.Mutex
}

var current atomic.Pointer[registry]

// Start loads the restrictions in force and keeps them fresh until ctx ends.
//
// A failed first load is returned for the caller to log, but the registry keeps
// running and retries on every tick. Until a load succeeds nobody is
// restricted — the API's behaviour before this feature existed — rather than
// an API that refuses to boot because a moderation list could not be read.
func Start(ctx context.Context, pool *pgxpool.Pool, every time.Duration) error {
	r := &registry{pool: pool}
	r.snap.Store(emptySnapshot())
	current.Store(r)

	err := r.reload(ctx)

	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if rerr := r.reload(ctx); rerr != nil && ctx.Err() == nil {
					log.Warn().Err(rerr).Msg("accountguard: refresh failed — keeping the previous snapshot")
				}
			}
		}
	}()
	return err
}

// Reload re-reads the restrictions in force. The console calls it straight
// after every change, so a ban applies to the account's very next request.
func Reload(ctx context.Context) error {
	r := current.Load()
	if r == nil {
		return nil
	}
	return r.reload(ctx)
}

func (r *registry) reload(ctx context.Context) error {
	// Serialised. Without the lock a slow tick that read the table BEFORE a
	// console change could finish AFTER the console's own reload and put the
	// older snapshot back — the ban would vanish for up to one interval.
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := r.pool.Query(qctx, `
		SELECT ar.id::text, ar.user_id::text, ar.kind, ar.ends_at, u.email
		FROM active_account_restrictions ar
		JOIN users u ON u.id = ar.user_id
	`)
	if err != nil {
		return fmt.Errorf("accountguard: load restrictions: %w", err)
	}
	defer rows.Close()

	next := emptySnapshot()
	for rows.Next() {
		var (
			id, userID, kind, email string
			endsAt                  *time.Time
		)
		if err := rows.Scan(&id, &userID, &kind, &endsAt, &email); err != nil {
			return fmt.Errorf("accountguard: scan restriction: %w", err)
		}
		k := models.RestrictionKind(kind)
		if !k.Valid() {
			// A kind this build does not know — written by a newer build or by
			// hand. Ignored rather than guessed at.
			continue
		}
		next.byUser[userID] = append(next.byUser[userID], Entry{ID: id, Kind: k, EndsAt: endsAt})
		if k.IsLockout() || k == models.RestrictBuyTickets {
			if e := NormalizeEmail(email); e != "" {
				next.byEmail[e] = userID
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("accountguard: read restrictions: %w", err)
	}

	r.snap.Store(next)
	return nil
}

func load() *snapshot {
	r := current.Load()
	if r == nil {
		return nil
	}
	return r.snap.Load()
}

// NormalizeEmail is the form emails are matched in.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ── Lookups ──────────────────────────────────────────────────────────────────

// LockFor returns the lockout in force for a user, if any.
func LockFor(userID string) (Lock, bool) {
	return lockAt(load(), userID, time.Now())
}

// FeatureBlock returns the block in force on one feature for a user, if any.
func FeatureBlock(userID string, feature models.RestrictionKind) (Entry, bool) {
	return featureBlockAt(load(), userID, feature, time.Now())
}

// FeatureBlocks returns every feature block in force for a user, in the order
// the popup lists them. Empty when the account is not limited.
func FeatureBlocks(userID string) []Entry {
	return featureBlocksAt(load(), userID, time.Now())
}

// PurchaseBlockedForEmail reports whether an email address belongs to an
// account that may not buy tickets: disabled, banned, or blocked from buying.
func PurchaseBlockedForEmail(email string) bool {
	s := load()
	if s == nil {
		return false
	}
	userID, ok := s.byEmail[NormalizeEmail(email)]
	if !ok {
		return false
	}
	now := time.Now()
	if _, locked := lockAt(s, userID, now); locked {
		return true
	}
	_, blocked := featureBlockAt(s, userID, models.RestrictBuyTickets, now)
	return blocked
}

// lockAt picks the lockout that governs: a ban outranks a disable, and between
// two disables (the console never creates two, but psql could) the later end
// wins.
func lockAt(s *snapshot, userID string, now time.Time) (Lock, bool) {
	if s == nil || userID == "" {
		return Lock{}, false
	}
	var best *Entry
	for i := range s.byUser[userID] {
		e := s.byUser[userID][i]
		if !e.Kind.IsLockout() || !e.inForce(now) {
			continue
		}
		if best == nil || outranks(e, *best) {
			pick := e
			best = &pick
		}
	}
	if best == nil {
		return Lock{}, false
	}
	return Lock{Kind: best.Kind, EndsAt: best.EndsAt}, true
}

func featureBlockAt(s *snapshot, userID string, feature models.RestrictionKind, now time.Time) (Entry, bool) {
	if s == nil || userID == "" {
		return Entry{}, false
	}
	var best *Entry
	for i := range s.byUser[userID] {
		e := s.byUser[userID][i]
		if e.Kind != feature || !e.inForce(now) {
			continue
		}
		if best == nil || outranks(e, *best) {
			pick := e
			best = &pick
		}
	}
	if best == nil {
		return Entry{}, false
	}
	return *best, true
}

func featureBlocksAt(s *snapshot, userID string, now time.Time) []Entry {
	out := []Entry{}
	for _, f := range models.RestrictionFeatures {
		if e, ok := featureBlockAt(s, userID, f, now); ok {
			out = append(out, e)
		}
	}
	return out
}

// outranks reports whether a governs over b: ban over disable, then an
// indefinite end over any date, then the later date.
func outranks(a, b Entry) bool {
	if a.Kind != b.Kind {
		return a.Kind == models.RestrictBan
	}
	if a.EndsAt == nil {
		return b.EndsAt != nil
	}
	if b.EndsAt == nil {
		return false
	}
	return a.EndsAt.After(*b.EndsAt)
}

// ── What the account holder is told ─────────────────────────────────────────

// DaysRemaining rounds up: 49 hours left reads "3 days", and a few minutes
// left reads "1 day", never "0 days".
func DaysRemaining(end, now time.Time) int {
	left := end.Sub(now)
	if left <= 0 {
		return 0
	}
	return int(math.Ceil(left.Hours() / 24))
}

// Message is the popup text for a lockout.
func (l Lock) Message(now time.Time) string {
	if l.Kind == models.RestrictBan {
		return "Your account has been banned."
	}
	days := 1
	if l.EndsAt != nil {
		if d := DaysRemaining(*l.EndsAt, now); d > 1 {
			days = d
		}
	}
	if days == 1 {
		return "Your account has been disabled for 1 day."
	}
	return fmt.Sprintf("Your account has been disabled for %d days.", days)
}

// Body is the JSON a locked-out account receives: 403 at sign-in, 401 on any
// other request.
func (l Lock) Body(now time.Time) gin.H {
	msg := l.Message(now)
	status := gin.H{
		"state":         "banned",
		"title":         "Account banned",
		"message":       msg,
		"detail":        "If you think this is a mistake, contact " + SupportEmail + ".",
		"until":         nil,
		"support_email": SupportEmail,
	}
	code := CodeBanned

	if l.Kind == models.RestrictDisable {
		code = CodeDisabled
		status["state"] = "disabled"
		status["title"] = "Account disabled"
		if l.EndsAt != nil {
			when := utils.FormatSAShortDateTime(*l.EndsAt)
			status["until"] = l.EndsAt.UTC()
			status["until_text"] = when
			status["days_remaining"] = DaysRemaining(*l.EndsAt, now)
			status["detail"] = "You can sign in again after " + when + "."
		}
	}

	return gin.H{"error": msg, "code": code, "account_status": status}
}

// AbortLocked ends the request with the lockout payload.
func AbortLocked(c *gin.Context, httpStatus int, l Lock) {
	c.AbortWithStatusJSON(httpStatus, l.Body(time.Now()))
}

// FeatureSentence is what one block means to the account holder.
func FeatureSentence(e Entry) string {
	var base string
	switch e.Kind {
	case models.RestrictBuyTickets:
		base = "You can't buy tickets"
	case models.RestrictCreateEvents:
		base = "You can't create or publish events"
	case models.RestrictBankDetails:
		base = "You can't change your bank details"
	case models.RestrictPayouts:
		base = "Your payouts are on hold"
	default:
		base = "Part of your account is unavailable"
	}
	if e.EndsAt != nil {
		return base + " until " + utils.FormatSAShortDateTime(*e.EndsAt) + "."
	}
	return base + "."
}

// LimitedStatus is the "account has been limited" status for a user, returned
// beside the tokens at sign-in. ok is false when nothing is blocked.
func LimitedStatus(userID string) (gin.H, bool) {
	blocks := FeatureBlocks(userID)
	if len(blocks) == 0 {
		return nil, false
	}
	return limitedBody(blocks), true
}

func limitedBody(blocks []Entry) gin.H {
	limits := make([]gin.H, 0, len(blocks))
	sentences := make([]string, 0, len(blocks))
	for _, b := range blocks {
		item := gin.H{"feature": b.Kind, "label": b.Kind.Label(), "until": nil}
		if b.EndsAt != nil {
			item["until"] = b.EndsAt.UTC()
			item["until_text"] = utils.FormatSAShortDateTime(*b.EndsAt)
		}
		limits = append(limits, item)
		sentences = append(sentences, FeatureSentence(b))
	}
	return gin.H{
		"state":         "limited",
		"title":         "Account limited",
		"message":       "Your account has been limited.",
		"detail":        strings.Join(sentences, " "),
		"limits":        limits,
		"support_email": SupportEmail,
	}
}

// FeatureBlockedBody is the 403 for an action the account is blocked from.
// account_status lists every block in force, so the popup can say everything
// at once instead of one refusal at a time.
func FeatureBlockedBody(userID string, blocked Entry) gin.H {
	blocks := FeatureBlocks(userID)
	if len(blocks) == 0 {
		// Lapsed or lifted between the check and here. Still explain the
		// refusal that actually happened.
		blocks = []Entry{blocked}
	}
	body := gin.H{
		"error":          "Your account has been limited. " + FeatureSentence(blocked),
		"code":           CodeLimited,
		"feature":        blocked.Kind,
		"until":          nil,
		"account_status": limitedBody(blocks),
	}
	if blocked.EndsAt != nil {
		body["until"] = blocked.EndsAt.UTC()
	}
	return body
}

// PurchaseBlockedBody is the 403 guest checkout returns for an email that
// belongs to an account which may not buy tickets.
//
// Deliberately says nothing about WHY. Anyone can type any address into guest
// checkout, and "this account is banned" would turn the form into a lookup of
// other people's standing. The account holder signs in and gets the real popup.
func PurchaseBlockedBody() gin.H {
	return gin.H{
		"error": "Tickets can't be bought with this email address. " +
			"If it belongs to your TicketHawk account, sign in to see why.",
		"code": CodePurchaseBlocked,
	}
}