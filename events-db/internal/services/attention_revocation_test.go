package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// REVOCATION TESTS — Phase 4c gate
// =============================================================================
//
//	go test ./internal/services/... -v -run "TestRevocation|TestSerial"
//
// The staleness policy and serial normalisation are the parts worth pinning:
// both fail silently when wrong, and both would leave a revoked device passing
// while everything looks healthy.

type fakeRevocationStore struct {
	listed      map[string]string
	lastSuccess time.Time
	failures    int
	lookupErr   error
}

func (f *fakeRevocationStore) Replace(_ context.Context, entries []RevocationEntry, at time.Time) error {
	f.listed = map[string]string{}
	for _, e := range entries {
		f.listed[e.Serial] = e.Status
	}
	f.lastSuccess = at
	return nil
}

func (f *fakeRevocationStore) IsListed(_ context.Context, serial string) (bool, string, error) {
	if f.lookupErr != nil {
		return false, "", f.lookupErr
	}
	s, ok := f.listed[serial]
	return ok, s, nil
}

func (f *fakeRevocationStore) LastSuccess(context.Context) (time.Time, error) {
	return f.lastSuccess, nil
}

func (f *fakeRevocationStore) RecordFailure(context.Context, time.Time, error) error {
	f.failures++
	return nil
}

var revNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func checkerWith(listed map[string]string, last time.Time) (*RevocationChecker, *fakeRevocationStore) {
	store := &fakeRevocationStore{listed: listed, lastSuccess: last}
	return NewRevocationChecker(store), store
}

// ─── The staleness window ────────────────────────────────────────────────────

func TestRevocationNeverFetchedRefuses(t *testing.T) {
	// A list that has never been fetched is not a control. Refusing is the only
	// honest answer — the alternative is enrolling devices against no check
	// while the code reads as though one exists.
	c, _ := checkerWith(nil, time.Time{})
	ok, reason, err := c.CheckChain(context.Background(), []string{"abc"}, revNow)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, reason, "never been fetched")
}

func TestRevocationStaleListRefuses(t *testing.T) {
	c, _ := checkerWith(nil, revNow.Add(-48*time.Hour))
	ok, reason, err := c.CheckChain(context.Background(), []string{"abc"}, revNow)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, reason, "stale")
}

func TestRevocationFreshListPasses(t *testing.T) {
	// Inside the window, a transient fetch failure must NOT take out enrolment
	// — the outage would be Google's, not yours.
	c, _ := checkerWith(map[string]string{}, revNow.Add(-6*time.Hour))
	ok, reason, err := c.CheckChain(context.Background(), []string{"abc"}, revNow)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestRevocationWindowBoundary(t *testing.T) {
	c, _ := checkerWith(map[string]string{}, revNow.Add(-24*time.Hour))

	ok, _, err := c.CheckChain(context.Background(), []string{"abc"}, revNow)
	require.NoError(t, err)
	assert.True(t, ok, "exactly MaxAge old is still usable")

	ok, _, err = c.CheckChain(context.Background(), []string{"abc"}, revNow.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, ok, "one minute past MaxAge refuses")
}

// ─── Listed certificates ─────────────────────────────────────────────────────

func TestRevocationListedSerialRefuses(t *testing.T) {
	for _, status := range []string{RevocationRevoked, RevocationSuspended} {
		c, _ := checkerWith(map[string]string{"abc123": status}, revNow.Add(-time.Hour))
		ok, reason, err := c.CheckChain(context.Background(), []string{"abc123"}, revNow)
		require.NoError(t, err)
		assert.False(t, ok, status)
		assert.Contains(t, reason, status)
	}
}

func TestRevocationChecksEveryCertNotJustTheLeaf(t *testing.T) {
	// Google lists intermediates as well as leaves, and a compromised
	// intermediate taints every device beneath it. Checking only the leaf would
	// miss a whole batch revoked together.
	c, _ := checkerWith(map[string]string{"deadbeef": RevocationRevoked}, revNow.Add(-time.Hour))

	ok, reason, err := c.CheckChain(context.Background(),
		[]string{"leafserial", "deadbeef", "rootserial"}, revNow)
	require.NoError(t, err)
	assert.False(t, ok, "a revoked intermediate must refuse the whole chain")
	assert.Contains(t, reason, "deadbeef")
}

func TestRevocationLookupErrorRefuses(t *testing.T) {
	// A database problem must not read as "not listed".
	store := &fakeRevocationStore{
		listed:      map[string]string{},
		lastSuccess: revNow.Add(-time.Hour),
		lookupErr:   errors.New("connection reset"),
	}
	c := NewRevocationChecker(store)
	ok, _, err := c.CheckChain(context.Background(), []string{"abc"}, revNow)
	assert.Error(t, err)
	assert.False(t, ok)
}

// ─── Serial normalisation ────────────────────────────────────────────────────

func TestSerialNormalisation(t *testing.T) {
	// The silent-failure case. Go's big.Int renders serials without leading
	// zeros while other tooling pads them; a mismatch means a revoked device
	// passes and everything looks healthy.
	for _, tc := range []struct{ in, want string }{
		{"ABC123", "abc123"},
		{"abc123", "abc123"},
		{"00abc123", "abc123"},
		{"0000ABC123", "abc123"},
		{"ab:c1:23", "abc123"},
		{"ab c1 23", "abc123"},
		{"ab-c1-23", "abc123"},
		{"  ABC123  ", "abc123"},
		{"0", "0"},
		{"0000", "0"},
		{"", "0"},
	} {
		assert.Equal(t, tc.want, NormaliseSerial(tc.in), "input %q", tc.in)
	}
}

func TestRevocationMatchesRegardlessOfSerialFormatting(t *testing.T) {
	// End to end: the list says one form, the certificate says another, and the
	// device must still be caught.
	c, _ := checkerWith(nil, time.Time{})
	store := &fakeRevocationStore{}
	c = NewRevocationChecker(store)

	require.NoError(t, store.Replace(context.Background(), []RevocationEntry{
		{Serial: NormaliseSerial("00ABC123"), Status: RevocationRevoked},
	}, revNow.Add(-time.Hour)))

	for _, presented := range []string{"abc123", "ABC123", "00abc123", "ab:c1:23"} {
		ok, _, err := c.CheckChain(context.Background(), []string{presented}, revNow)
		require.NoError(t, err)
		assert.False(t, ok, "presented as %q", presented)
	}
}

// ─── Cache replacement ───────────────────────────────────────────────────────

func TestRevocationReplaceUpdatesFreshness(t *testing.T) {
	store := &fakeRevocationStore{}
	c := NewRevocationChecker(store)

	// Before any fetch, refuse.
	ok, _, err := c.CheckChain(context.Background(), []string{"abc"}, revNow)
	require.NoError(t, err)
	assert.False(t, ok)

	// After a successful replace, allow.
	require.NoError(t, store.Replace(context.Background(), []RevocationEntry{}, revNow))
	ok, _, err = c.CheckChain(context.Background(), []string{"abc"}, revNow)
	require.NoError(t, err)
	assert.True(t, ok, "an empty list is a valid answer — it means nothing is revoked")
}