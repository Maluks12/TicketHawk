package repository

import (
	"context"
	"errors"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Attestation persistence ───────────────────────────────────────────────────
//
// Backs migrations 000088 and 000089. Three responsibilities that share a
// table space but not much else: the revocation cache, single-use nonces, and
// the record of what was attested and what key was released as a result.

var (
	ErrNonceUnknown  = errors.New("attestation nonce is unknown")
	ErrNonceConsumed = errors.New("attestation nonce has already been used")
	ErrNonceExpired  = errors.New("attestation nonce has expired")
)

// ── Revocation cache ─────────────────────────────────────────────────────────
//
// Plain functions taking primitives, NOT an implementation of
// services.RevocationStore — this package cannot import services, because
// services already imports repository and Go would reject the cycle. The
// adapter that satisfies the interface lives in services alongside the
// interface it satisfies, which is the right place for it anyway.

// ReplaceRevocations swaps the whole cached list in one transaction.
//
// Delete-then-insert rather than upsert-and-prune, because the list is
// authoritative in full: a serial that has dropped off it is no longer revoked,
// and merging would leave stale entries refusing devices Google has cleared.
// The transaction means a concurrent reader never sees a half-empty list and
// wrongly concludes nothing is revoked.
func ReplaceRevocations(
	ctx context.Context, pool *pgxpool.Pool,
	serials, statuses, reasons []string, fetchedAt time.Time,
) error {
	if len(serials) != len(statuses) || len(serials) != len(reasons) {
		return errors.New("revocation entry slices are not the same length")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM attestation_revocations`); err != nil {
		return err
	}
	for i := range serials {
		if _, err := tx.Exec(ctx, `
			INSERT INTO attestation_revocations (serial_number, status, reason, fetched_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (serial_number) DO UPDATE
			SET status = EXCLUDED.status, reason = EXCLUDED.reason, fetched_at = EXCLUDED.fetched_at
		`, serials[i], statuses[i], reasons[i], fetchedAt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE attestation_revocation_meta
		SET last_success_at = $1, last_attempt_at = $1, last_error = NULL, entry_count = $2
		WHERE id = TRUE
	`, fetchedAt, len(serials)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IsRevoked reports whether a serial is listed.
func IsRevoked(ctx context.Context, pool *pgxpool.Pool, serial string) (bool, string, error) {
	var status string
	err := pool.QueryRow(ctx,
		`SELECT status FROM attestation_revocations WHERE serial_number = $1`, serial).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, status, nil
}

// LastRevocationFetch returns the last SUCCESSFUL fetch time, or zero.
func LastRevocationFetch(ctx context.Context, pool *pgxpool.Pool) (time.Time, error) {
	var t *time.Time
	err := pool.QueryRow(ctx,
		`SELECT last_success_at FROM attestation_revocation_meta WHERE id = TRUE`).Scan(&t)
	if err != nil || t == nil {
		return time.Time{}, err
	}
	return *t, nil
}

// RecordRevocationFailure notes a failed refresh.
//
// last_success_at is deliberately untouched: a failed fetch does not make the
// cached list any older than it already is, and overwriting freshness here
// would turn one bad fetch into refused enrolment across every till.
func RecordRevocationFailure(ctx context.Context, pool *pgxpool.Pool, at time.Time, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	_, err := pool.Exec(ctx, `
		UPDATE attestation_revocation_meta
		SET last_attempt_at = $1, last_error = $2 WHERE id = TRUE
	`, at, msg)
	return err
}

// ── Nonces ───────────────────────────────────────────────────────────────────

// IssueAttestationNonce stores a challenge for a device.
//
// ttl should be short — a couple of minutes. The nonce is what stops a device
// replaying attestation evidence captured from a clean handset, and a long
// window is a long window for that.
func IssueAttestationNonce(ctx context.Context, pool *pgxpool.Pool, deviceID, nonce string, ttl time.Duration) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO pos_attestation_nonces (nonce, device_id, expires_at)
		VALUES ($1, $2, now() + $3::interval)
	`, nonce, deviceID, ttl.String())
	return err
}

// ConsumeAttestationNonce marks a nonce used and reports whether it was valid.
//
// The UPDATE ... WHERE consumed_at IS NULL is the whole control: two concurrent
// enrolments presenting the same nonce cannot both win, because the database
// decides. A read-then-write here would let a replay through on the losing
// branch, which is exactly the race an attacker with a captured token would try.
//
// Rows are consumed rather than deleted so a replay ATTEMPT stays visible.
func ConsumeAttestationNonce(ctx context.Context, pool *pgxpool.Pool, deviceID, nonce string) error {
	var expiresAt time.Time
	var consumedAt *time.Time
	err := pool.QueryRow(ctx, `
		SELECT expires_at, consumed_at FROM pos_attestation_nonces
		WHERE nonce = $1 AND device_id = $2
	`, nonce, deviceID).Scan(&expiresAt, &consumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNonceUnknown
	}
	if err != nil {
		return err
	}
	if consumedAt != nil {
		return ErrNonceConsumed
	}
	if time.Now().After(expiresAt) {
		return ErrNonceExpired
	}

	res, err := pool.Exec(ctx, `
		UPDATE pos_attestation_nonces SET consumed_at = now()
		WHERE nonce = $1 AND device_id = $2 AND consumed_at IS NULL
	`, nonce, deviceID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		// Lost the race to a concurrent request presenting the same nonce.
		return ErrNonceConsumed
	}
	return nil
}

// PurgeExpiredNonces is housekeeping for the scheduler. Keeps consumed rows for
// a week so a replay attempt remains investigable after the event.
func PurgeExpiredNonces(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	res, err := pool.Exec(ctx, `
		DELETE FROM pos_attestation_nonces
		WHERE expires_at < now() - interval '7 days'
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

// ── Attestation records ──────────────────────────────────────────────────────

// RecordAttestation writes the outcome, pass or fail.
//
// Failures matter more than passes here: a till that fails three times in an
// hour is a device somebody is working on, and that pattern is invisible if
// only successes are kept.
func RecordAttestation(
	ctx context.Context, pool *pgxpool.Pool,
	deviceID, eventID, verdict, securityLevel string, detail map[string]any,
) (string, error) {
	if detail == nil {
		detail = map[string]any{}
	}
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO pos_device_attestations (device_id, event_id, verdict, security_level, detail)
		VALUES ($1, $2, $3::attestation_verdict, $4, $5)
		RETURNING id
	`, deviceID, eventID, verdict, nullIfEmpty(securityLevel), detail).Scan(&id)
	if err != nil {
		return "", err
	}

	if verdict != "passed" {
		// Counter drives the "device somebody is working on" alert. Reset on a
		// pass below, so it measures a run of failures rather than a lifetime
		// total — one failed attestation months ago is not a signal.
		_, _ = pool.Exec(ctx, `
			UPDATE vendor_pos_devices
			SET attestation_status = $2,
			    attestation_fail_count = attestation_fail_count + 1,
			    offline_enabled = FALSE
			WHERE id = $1
		`, deviceID, verdict)
	}
	return id, nil
}

// MarkDeviceAttested records a pass and enables offline mode.
//
// chk_pos_offline_requires_attestation refuses this write unless the status is
// 'passed' AND a signing key is present, so the constraint and this function
// have to agree — which is the point of setting all three in one statement.
func MarkDeviceAttested(
	ctx context.Context, pool *pgxpool.Pool,
	deviceID, signingPubKey string,
) error {
	res, err := pool.Exec(ctx, `
		UPDATE vendor_pos_devices
		SET attestation_status = 'passed',
		    attested_at = now(),
		    attestation_fail_count = 0,
		    signing_pubkey = $2,
		    offline_enabled = TRUE
		WHERE id = $1 AND status = 'active'
	`, deviceID, signingPubKey)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("device not found or not active")
	}
	return nil
}

// RevokeDeviceOffline turns a till back to online-only. Used when attestation
// later fails, when a device is reported lost, or when an operator pulls it.
func RevokeDeviceOffline(ctx context.Context, pool *pgxpool.Pool, deviceID, reason string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE vendor_pos_devices SET offline_enabled = FALSE WHERE id = $1
	`, deviceID); err != nil {
		return err
	}
	// Any live key grant dies with it. Leaving a grant valid while offline mode
	// is off would mean a device that still holds a usable event key.
	if _, err := tx.Exec(ctx, `
		UPDATE pos_key_grants
		SET revoked_at = now(), revoke_reason = $2
		WHERE device_id = $1 AND revoked_at IS NULL
	`, deviceID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ── Key grants ───────────────────────────────────────────────────────────────

// RecordKeyGrant logs a released key. The material itself is never stored —
// only the fact, so a compromise can be scoped to specific devices and versions.
func RecordKeyGrant(
	ctx context.Context, pool *pgxpool.Pool,
	deviceID, eventID, attestationID string, keyVersion int, expiresAt time.Time,
) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO pos_key_grants (device_id, event_id, attestation_id, key_version, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, deviceID, eventID, attestationID, keyVersion, expiresAt).Scan(&id)
	return id, err
}

// DeviceHasLiveGrant reports whether a device already holds an unexpired key.
// Lets enrolment be idempotent: a till that reconnects mid-shift should not
// force a fresh attestation round if what it has is still valid.
func DeviceHasLiveGrant(ctx context.Context, pool *pgxpool.Pool, deviceID string, keyVersion int) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pos_key_grants
			WHERE device_id = $1 AND key_version = $2
			  AND revoked_at IS NULL AND expires_at > now()
		)
	`, deviceID, keyVersion).Scan(&exists)
	return exists, err
}

// ListDeviceAttestations powers the organiser's device health view.
func ListDeviceAttestations(ctx context.Context, pool *pgxpool.Pool, eventID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := pool.Query(ctx, `
		SELECT a.id, a.device_id, d.label, v.name, a.verdict::text,
		       a.security_level, a.detail, a.attested_at
		FROM pos_device_attestations a
		JOIN vendor_pos_devices d ON d.id = a.device_id
		LEFT JOIN event_vendors v ON v.id = d.vendor_id
		WHERE a.event_id = $1
		ORDER BY a.attested_at DESC
		LIMIT $2
	`, eventID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var (
			id, deviceID, label, verdict string
			vendorName, securityLevel    *string
			detail                       map[string]any
			at                           time.Time
		)
		if err := rows.Scan(&id, &deviceID, &label, &vendorName, &verdict,
			&securityLevel, &detail, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "device_id": deviceID, "label": label,
			"vendor": vendorName, "verdict": verdict,
			"security_level": securityLevel, "detail": detail, "attested_at": at,
		})
	}
	return out, rows.Err()
}

// ── Event policy lookups ─────────────────────────────────────────────────────

// EventOfflineEnabled reports whether offline spend is on for an event, and
// which tag key version its bands are encoded under.
//
// A MISSING event_cashless_config row means DISABLED. That is the safe default
// established in 000087 and it must never be read as "use the built-in caps" —
// an organiser who never configured offline spend gets an event where no till
// ever holds a key.
func EventOfflineEnabled(ctx context.Context, pool *pgxpool.Pool, eventID string) (bool, int, error) {
	var policy string
	err := pool.QueryRow(ctx, `
		SELECT offline_policy::text FROM event_cashless_config WHERE event_id = $1
	`, eventID).Scan(&policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if policy != "enabled" {
		return false, 0, nil
	}

	versions, err := EventKeyVersions(ctx, pool, eventID)
	if err != nil {
		return false, 0, err
	}
	// Newest version first, from EventKeyVersions. A device enrolling mid-
	// rotation gets the current key; bands on the old version keep verifying
	// against whatever their own tags were encoded with.
	return true, versions[0], nil
}

// EventScanCloses returns the end of the event's admission window, used as the
// expiry on a key grant.
//
// Scoping the grant to the window is what makes a stolen till a bounded
// problem: the key it holds stops being useful when the event does. Mirrors how
// ScannerCredentialService scopes the gate credential.
func EventScanCloses(ctx context.Context, pool *pgxpool.Pool, eventID string) (time.Time, error) {
	var (
		startTime, endTime         time.Time
		scanOpenOff, scanCloseOff  *int
	)
	if err := pool.QueryRow(ctx, `
		SELECT start_time, end_time, scan_open_offset_min, scan_close_offset_min
		FROM events WHERE id = $1
	`, eventID).Scan(&startTime, &endTime, &scanOpenOff, &scanCloseOff); err != nil {
		return time.Time{}, err
	}
	_, scanClose := models.ScanWindow(startTime, endTime, scanOpenOff, scanCloseOff)
	return scanClose, nil
}