-- ============================================================================
-- FIX_000068.sql — repair the wrongly-typed staff_role AND make migrate re-run
--
-- WHY THE VERSION ROLLBACK IS NEEDED
--
-- The first 000068 wrapped CREATE TYPE in a DO block that swallowed
-- duplicate_object. Because 000050 had already taken the name `staff_role`,
-- that guard turned a hard collision into a no-op — so the migration ran to
-- completion WITHOUT ERROR and golang-migrate recorded version 68 as clean.
--
-- That means simply pushing the corrected file changes nothing: migrate sees 68
-- as already applied and skips it. The version has to go back to 67 for the
-- corrected migration to run at all.
--
-- Run this BEFORE restarting the api container:
--
--   docker run --rm -i --env-file ./.env.production postgres:16-alpine \
--     sh -c 'psql "$DATABASE_URL" -v ON_ERROR_STOP=1' < FIX_000068.sql
--
-- ON_ERROR_STOP=1 is not optional. Without it psql continues past a failure and
-- exits 0, which is how a half-repaired database reports success.
-- ============================================================================

BEGIN;

-- ── 1. Objects the bad migration created ────────────────────────────────────
--
-- All of these were built against the wrong enum. Nothing here can hold real
-- data: the console was never reachable, so no staff action was ever recorded.
--
-- The audit log's append-only RULEs are dropped explicitly first. DROP TABLE
-- would remove them anyway, but doing it as its own statement keeps the
-- destruction of that protection a visible line rather than a side effect.
DROP RULE IF EXISTS staff_audit_log_no_update ON staff_audit_log;
DROP RULE IF EXISTS staff_audit_log_no_delete ON staff_audit_log;

DROP TABLE IF EXISTS staff_audit_log;
DROP TABLE IF EXISTS staff_request_confirmations;
DROP TABLE IF EXISTS staff_requests;

DROP TYPE IF EXISTS staff_request_status;
DROP TYPE IF EXISTS staff_request_kind;

-- ── 2. The mistyped column ──────────────────────────────────────────────────
--
-- Conditional on it actually being the door-staff enum, so re-running this
-- after a partial attempt is harmless.
--
-- No data is lost. The column could only ever have held 'scanner' or 'manager',
-- which is precisely why the grant failed.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'users'
          AND column_name = 'staff_role'
          AND udt_name = 'staff_role'      -- the 000050 door-staff enum
    ) THEN
        DROP INDEX IF EXISTS idx_users_staff_role;
        ALTER TABLE users DROP COLUMN staff_role;
        RAISE NOTICE 'Dropped users.staff_role (was bound to the door-staff enum)';
    ELSE
        RAISE NOTICE 'users.staff_role absent or already correct — nothing to drop';
    END IF;
END $$;

-- DELIBERATELY NOT DROPPED: TYPE staff_role.
-- It belongs to migration 000050 and is still in use by event_staff and
-- event_staff_invites. Dropping it breaks door scanning at every event.

-- ── 3. Payout-hold columns ──────────────────────────────────────────────────
-- Removed so the corrected migration applies to a clean slate rather than a
-- mixture of old and new.
ALTER TABLE organizers
    DROP COLUMN IF EXISTS payout_hold_until,
    DROP COLUMN IF EXISTS payout_hold_reason;

-- ── 4. Roll the recorded version back to 67 ─────────────────────────────────
--
-- Only if it is currently 68. Guarded so running this twice cannot walk the
-- version backwards through migrations that are legitimately applied.
--
-- dirty = false matters as much as the version: migrate refuses to run at all
-- against a dirty schema and requires a manual force, so leaving it true would
-- swap one stuck state for another.
DO $$
DECLARE
    v BIGINT;
BEGIN
    SELECT version INTO v FROM schema_migrations LIMIT 1;
    IF v = 68 THEN
        UPDATE schema_migrations SET version = 67, dirty = FALSE;
        RAISE NOTICE 'schema_migrations rolled back 68 -> 67; corrected 000068 will re-run';
    ELSE
        RAISE NOTICE 'schema_migrations is at %, leaving it alone', v;
    END IF;
END $$;

COMMIT;

-- ── 5. Verify ───────────────────────────────────────────────────────────────
SELECT 'schema version (want 67, dirty f)' AS check,
       version::text || ', dirty=' || dirty::text AS value
FROM schema_migrations
UNION ALL
SELECT 'staff_role enum (000050 — MUST survive as scanner,manager)',
       coalesce(string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder), 'MISSING — problem')
FROM pg_type t LEFT JOIN pg_enum e ON e.enumtypid = t.oid
WHERE t.typname = 'staff_role'
UNION ALL
SELECT 'users.staff_role column (want absent)',
       coalesce(string_agg(column_name, ','), 'absent')
FROM information_schema.columns
WHERE table_name = 'users' AND column_name = 'staff_role'
UNION ALL
SELECT 'staff_requests table (want absent)',
       coalesce(string_agg(table_name, ','), 'absent')
FROM information_schema.tables WHERE table_name = 'staff_requests';