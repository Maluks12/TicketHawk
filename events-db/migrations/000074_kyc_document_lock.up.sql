-- ============================================================================
-- 000074: Lock identity documents after the first completed KYC review, and
--         bootstrap console access for pre-existing platform admins.
--
-- ── PART 1: THE ONE-WAY LATCH ───────────────────────────────────────────────
--
-- The rule this exists to express:
--
--   BEFORE the first completed review   the reviewing staff member may look at
--                                       the ID photo, because you cannot verify
--                                       an identity you are not allowed to see
--   AFTER  the first completed review   MASTER ADMIN ONLY, forever
--
-- Why a new column rather than reading kyc_status:
--
-- kyc_status moves. An organiser who is verified today can be set back to
-- pending_review by a support action, a re-verification, or a data fix — and
-- the moment it moves, a status-based rule silently reopens the document to
-- every editor on the team. That is a permission that un-revokes itself, which
-- is the worst kind.
--
-- kyc_first_verified_at only ever goes from NULL to a timestamp. COALESCE on
-- write makes it a latch: once the document has done its job, nothing short of
-- a deliberate UPDATE re-opens it, and that UPDATE is visible in a way a status
-- flip is not.
--
-- The break-glass path is unchanged: an editor or admin who genuinely needs a
-- verified organiser's document asks for a grant (000073) and a master admin
-- approves it, scoped to that one organiser and expiring on its own.
-- ============================================================================

ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS kyc_first_verified_at TIMESTAMPTZ;

COMMENT ON COLUMN organizers.kyc_first_verified_at IS
    'When KYC completed for the FIRST time. Write-once latch: set by COALESCE '
    'so a later re-verification never clears it. Once set, the identity '
    'document is readable by master admins only (or by an approved, expiring '
    'grant from kyc_document_grants). Distinct from kyc_verified_at, which '
    'tracks the CURRENT verification and can move.';

-- Backfill: anyone already verified has, by definition, had their first review.
-- Without this, every organiser verified before today would keep an open
-- document until someone re-verified them — the migration would ship the rule
-- and exempt exactly the accounts that have been on the platform longest.
UPDATE organizers
   SET kyc_first_verified_at = COALESCE(kyc_first_verified_at, kyc_verified_at)
 WHERE kyc_verified_at IS NOT NULL
   AND kyc_first_verified_at IS NULL;

-- Belt and braces for rows that reached 'verified' without a timestamp (older
-- manual fixes). Their submission time is the closest honest answer, and the
-- alternative — leaving the latch unset — leaves the document open.
UPDATE organizers
   SET kyc_first_verified_at = COALESCE(kyc_submitted_at, now())
 WHERE kyc_status = 'verified'
   AND kyc_first_verified_at IS NULL;

-- Answers "is this document still in its review window" without a table scan.
CREATE INDEX IF NOT EXISTS idx_organizers_kyc_first_verified
    ON organizers (id) WHERE kyc_first_verified_at IS NOT NULL;

-- ============================================================================
-- ── PART 2: CONSOLE BOOTSTRAP ───────────────────────────────────────────────
--
-- 000068 added users.staff_role and deliberately left it NULL for every
-- existing account, so console access would be granted rather than inherited.
-- Correct in principle, and it has one consequence nobody wants:
--
--   /admin/me/capabilities sits behind RequireStaff, which refuses a NULL
--   staff_role. The console fails closed on an empty capability list. So the
--   platform's own admins — including whoever must log in to grant the first
--   staff_role — see "Access denied" on Analytics, Approvals, Staff Requests
--   and People. The console locks its own key inside.
--
-- Granting staff_role='admin' to accounts that ALREADY hold users.role='admin'
-- escalates nothing: those accounts can call every legacy admin endpoint today,
-- unrestricted. This narrows what they can do, it does not widen it.
--
-- It does NOT grant is_master_admin. That stays a separate, hand-set column, so
-- this backfill cannot hand anyone an identity document.
-- ============================================================================

UPDATE users
   SET staff_role = 'admin'::console_role
 WHERE role = 'admin'
   AND staff_role IS NULL;

-- ── Master admin safety net ─────────────────────────────────────────────────
--
-- If NOBODY holds is_master_admin, verified organisers' documents become
-- unreadable by anyone at all and no grant can ever be approved — the system
-- would be locked shut rather than locked down.
--
-- This does not pick a winner. It raises a loud NOTICE so the operator sets it
-- deliberately:
--
--   UPDATE users SET is_master_admin = TRUE, master_admin_granted_at = now()
--    WHERE email = 'you@example.com';
DO $$
DECLARE
    n INTEGER;
BEGIN
    SELECT COUNT(*) INTO n FROM users WHERE is_master_admin = TRUE;
    IF n = 0 THEN
        RAISE NOTICE '';
        RAISE NOTICE '  ⚠  NO MASTER ADMIN IS SET.';
        RAISE NOTICE '     Identity documents for verified organisers are now';
        RAISE NOTICE '     unreadable, and document access grants cannot be';
        RAISE NOTICE '     approved, until one account holds is_master_admin.';
        RAISE NOTICE '';
        RAISE NOTICE '     UPDATE users SET is_master_admin = TRUE,';
        RAISE NOTICE '            master_admin_granted_at = now()';
        RAISE NOTICE '      WHERE email = ''<your address>'';';
        RAISE NOTICE '';
    END IF;
END $$;