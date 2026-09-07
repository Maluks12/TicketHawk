package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Attestation revocation ────────────────────────────────────────────────────
//
// Closes the gap the fixes drop left open.
//
// A device whose attestation key has been EXTRACTED from its hardware produces
// a chain that verifies perfectly to a pinned Google root — because the key
// genuinely was issued by Google. It has simply been prised out of the secure
// element it was meant to live in, and can now sign attestations for software
// keys on any machine. Chain verification cannot see that. This can.
//
// Which makes revocation checking not an optional hardening step but the thing
// that stops the most deliberately compromised class of device from being the
// one class that passes.
//
// Google publishes the list at https://android.googleapis.com/attestation/status
// in the shape:
//
//	{"entries": {"<serial-hex>": {"status": "REVOKED", "reason": "KEY_COMPROMISE"}}}
//
// Serials are lowercase hex with no leading zeros.

const revocationListURL = "https://android.googleapis.com/attestation/status"

// RevocationStatus values Google uses. Both refuse — SUSPENDED means Google is
// investigating, which is not a state to be handing out event keys during.
const (
	RevocationRevoked   = "REVOKED"
	RevocationSuspended = "SUSPENDED"
)

// RevocationEntry is one listed serial.
type RevocationEntry struct {
	Serial string
	Status string
	Reason string
}

// RevocationStore is the persistence the checker needs. An interface so the
// policy below is testable without a database, matching how the verifiers are
// structured.
type RevocationStore interface {
	Replace(ctx context.Context, entries []RevocationEntry, fetchedAt time.Time) error
	IsListed(ctx context.Context, serial string) (listed bool, status string, err error)
	LastSuccess(ctx context.Context) (time.Time, error)
	RecordFailure(ctx context.Context, at time.Time, err error) error
}

// RevocationChecker fetches, caches, and consults the list.
type RevocationChecker struct {
	store RevocationStore
	http  *http.Client

	// MaxAge is how stale the cached list may be before enrolment refuses.
	//
	// The two failure modes this balances: refusing the moment a fetch fails
	// takes out enrolment for an outage that is Google's, not yours; never
	// refusing means a list that quietly stopped refreshing months ago is still
	// treated as a live control. A day is long enough to ride out a transient
	// problem and short enough that a broken refresh loop surfaces before it
	// matters.
	MaxAge time.Duration
}

func NewRevocationChecker(store RevocationStore) *RevocationChecker {
	return &RevocationChecker{
		store:  store,
		http:   &http.Client{Timeout: 20 * time.Second},
		MaxAge: 24 * time.Hour,
	}
}

// Refresh pulls the list and replaces the cache.
//
// Call on a timer — hourly is ample, the list changes rarely — and once at
// startup. Failures are recorded rather than returned fatally, because a failed
// refresh is not an outage until the cache goes stale.
func (r *RevocationChecker) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, revocationListURL, nil)
	if err != nil {
		return err
	}

	resp, err := r.http.Do(req)
	if err != nil {
		_ = r.store.RecordFailure(ctx, time.Now(), err)
		return fmt.Errorf("fetching revocation list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		e := fmt.Errorf("revocation list returned %d", resp.StatusCode)
		_ = r.store.RecordFailure(ctx, time.Now(), e)
		return e
	}

	var payload struct {
		Entries map[string]struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		_ = r.store.RecordFailure(ctx, time.Now(), err)
		return fmt.Errorf("decoding revocation list: %w", err)
	}

	// A missing or absent "entries" key is not the same as an empty list, and
	// treating it as empty would wipe the cache on a malformed response —
	// turning a bad payload into a silently disabled control.
	if payload.Entries == nil {
		e := fmt.Errorf("revocation list contained no entries object")
		_ = r.store.RecordFailure(ctx, time.Now(), e)
		return e
	}

	entries := make([]RevocationEntry, 0, len(payload.Entries))
	for serial, v := range payload.Entries {
		entries = append(entries, RevocationEntry{
			Serial: NormaliseSerial(serial),
			Status: strings.ToUpper(v.Status),
			Reason: v.Reason,
		})
	}
	return r.store.Replace(ctx, entries, time.Now())
}

// CheckChain reports whether any certificate in an attestation chain is listed.
//
// EVERY serial is checked, not just the leaf. A compromised intermediate taints
// everything beneath it, and Google lists intermediates as well as leaves —
// checking only the leaf would miss a whole batch of devices revoked together.
func (r *RevocationChecker) CheckChain(ctx context.Context, serials []string, now time.Time) (ok bool, reason string, err error) {
	last, err := r.store.LastSuccess(ctx)
	if err != nil {
		return false, "revocation cache unavailable", err
	}

	// Never fetched, or too old to be a control any more.
	if last.IsZero() {
		return false, "revocation list has never been fetched", nil
	}
	if now.Sub(last) > r.MaxAge {
		return false, fmt.Sprintf("revocation list is stale (last fetched %s ago)",
			now.Sub(last).Truncate(time.Minute)), nil
	}

	for _, s := range serials {
		listed, status, err := r.store.IsListed(ctx, NormaliseSerial(s))
		if err != nil {
			return false, "revocation lookup failed", err
		}
		if listed {
			return false, fmt.Sprintf("certificate %s is %s", s, status), nil
		}
	}
	return true, "", nil
}

// NormaliseSerial canonicalises a certificate serial to the form Google
// publishes: lowercase hex, no leading zeros, no separators.
//
// Worth its own function and its own tests. Go's big.Int renders serials
// without leading zeros while some tooling pads them, and a formatting
// mismatch here means a revoked device silently passes — a failure that looks
// exactly like a working system.
func NormaliseSerial(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(":", "", " ", "", "-", "").Replace(s)
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0" // a serial of zero is legal, if unusual
	}
	return s
}

// ── Postgres adapter ─────────────────────────────────────────────────────────
//
// Lives here rather than in the repository package because repository cannot
// import services — services already imports repository, and Go rejects the
// cycle. Putting the adapter next to the interface it satisfies is the right
// side of that constraint anyway: the repository stays a plain data layer with
// no knowledge of who consumes it.

// PgRevocationStore implements RevocationStore over the tables in 000089.
type PgRevocationStore struct{ Pool *pgxpool.Pool }

func NewPgRevocationStore(pool *pgxpool.Pool) *PgRevocationStore {
	return &PgRevocationStore{Pool: pool}
}

func (s *PgRevocationStore) Replace(ctx context.Context, entries []RevocationEntry, fetchedAt time.Time) error {
	serials := make([]string, len(entries))
	statuses := make([]string, len(entries))
	reasons := make([]string, len(entries))
	for i, e := range entries {
		serials[i], statuses[i], reasons[i] = e.Serial, e.Status, e.Reason
	}
	return repository.ReplaceRevocations(ctx, s.Pool, serials, statuses, reasons, fetchedAt)
}

func (s *PgRevocationStore) IsListed(ctx context.Context, serial string) (bool, string, error) {
	return repository.IsRevoked(ctx, s.Pool, serial)
}

func (s *PgRevocationStore) LastSuccess(ctx context.Context) (time.Time, error) {
	return repository.LastRevocationFetch(ctx, s.Pool)
}

func (s *PgRevocationStore) RecordFailure(ctx context.Context, at time.Time, cause error) error {
	return repository.RecordRevocationFailure(ctx, s.Pool, at, cause)
}

// StartRevocationRefresh runs the refresh loop.
//
// Hourly is ample — the list changes rarely — and it refreshes once immediately
// so a freshly deployed server is not sitting on an empty cache refusing every
// enrolment. Failures are logged and swallowed: a failed refresh is not an
// outage until the cache goes stale, which CheckChain enforces separately.
func StartRevocationRefresh(ctx context.Context, checker *RevocationChecker, every time.Duration) {
	if every <= 0 {
		every = time.Hour
	}
	go func() {
		if err := checker.Refresh(ctx); err != nil {
			log.Warn().Err(err).Msg("attestation: initial revocation refresh failed")
		}
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := checker.Refresh(ctx); err != nil {
					log.Warn().Err(err).Msg("attestation: revocation refresh failed")
				}
			}
		}
	}()
}