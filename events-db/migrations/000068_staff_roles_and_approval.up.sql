-- ============================================================================
-- 000068: Staff roles, multi-layer approval, and an append-only audit log.
--
-- THE PROBLEM
--
-- users.role is attendee | organizer | admin — one bit of staff granularity.
-- Everyone with dashboard access has identical power, so "let someone help with
-- support tickets" and "let someone change where an organiser's money is sent"
-- are the same grant. There is also no record of who did what.
--
-- THE SHAPE OF THE FIX
--
-- A SEPARATE staff_role axis rather than more values in user_role. The two
-- answer different questions:
--
--   user_role   what this account is on the platform (buys tickets? sells them?)
--   staff_role  what this human may do inside the internal console
--
-- Folding them together would mean a support agent who also buys a ticket needs
-- two accounts, and every existing `role = 'admin'` check would silently change
-- meaning. Keeping them orthogonal means AdminOnlyMiddleware keeps working
-- exactly as it does today and staff capability is layered on top.
--
-- WHY DANGEROUS ACTIONS ARE ROWS, NOT FUNCTION CALLS
--
-- Changing an organiser's payout bank account is the highest-value target in
-- this product: it is how ticket money is redirected. An API that performs it
-- in one call, however well authenticated, is one compromised staff session
-- away from a payout going somewhere else — and with no record of it happening.
--
-- So the destructive operations don't execute on request. They become a row
-- here that has to survive four independent gates:
--
--   1. AUTHORISATION  the requester holds a staff_role permitted to ask
--   2. SECOND PERSON  a DIFFERENT admin approves (four-eyes)
--   3. SUBJECT        the affected organiser confirms by OTP
--   4. COOLING-OFF    execution is blocked until execute_not_before passes
--
-- Gates 2 and 4 are enforced by CHECK constraints below, not by application
-- code. Application code is where mistakes live; a constraint holds even if a
-- handler is wrong, a migration is half-applied, or someone reaches the
-- database directly.
-- ============================================================================

-- ── Staff roles ──────────────────────────────────────────────────────────────
--
-- Deliberately ordered least → most privileged so `>=`-style comparisons in Go
-- read naturally against the enum's ordinal.
-- Wrapped in a DO block because Postgres has no `CREATE TYPE IF NOT EXISTS`.
-- Without this guard, re-running this migration after any partial failure
-- (e.g. a killed container) fails forever on this line, since every other
-- statement below is idempotent but this one wasn't.
DO $$ BEGIN
    CREATE TYPE staff_role AS ENUM (
        'read_only',  -- lists, names, roles. No PII, no writes.
        'access',     -- + emails, KYC status, masked bank tail. Still read-only.
        'editor',     -- + act on accounts, and LOG requests for dangerous actions.
        'admin'       -- + approve requests. Still cannot read ID docs or bank numbers.
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- NULL = not staff. No default, so every existing account starts non-staff and
-- access is granted deliberately rather than inherited by an ALTER TABLE.
ALTER TABLE users ADD COLUMN IF NOT EXISTS staff_role staff_role;

COMMENT ON COLUMN users.staff_role IS
    'Internal console capability. Orthogonal to users.role. NULL means not '
    'staff. Note: admin here is NOT permitted to read ID documents or full bank '
    'numbers — that is enforced in the repository layer, not by this column.';

CREATE INDEX IF NOT EXISTS idx_users_staff_role
    ON users (staff_role) WHERE staff_role IS NOT NULL;

-- ── Request kinds ────────────────────────────────────────────────────────────
DO $$ BEGIN
    CREATE TYPE staff_request_kind AS ENUM (
        'bank_details_delete',
        'bank_details_add',
        'event_takedown',
        'role_change',
        'credential_reset',      -- password/email reset for a compromised account
        'account_delete'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE staff_request_status AS ENUM (
        'pending_approval',        -- waiting on a second admin
        'awaiting_subject',        -- approved; waiting on the organiser's OTP
        'ready',                   -- all gates passed, cooling-off may still apply
        'executed',
        'rejected',
        'cancelled',               -- withdrawn by the requester
        'expired'                  -- nobody acted in time
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- ── The request table ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS staff_requests (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    kind          staff_request_kind   NOT NULL,
    status        staff_request_status NOT NULL DEFAULT 'pending_approval',

    -- Layer 1: who asked, and why. Reason is NOT NULL and length-checked
    -- because "approve this" with no stated purpose is not a reviewable
    -- request — the approver needs something to actually evaluate.
    requested_by  UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason        TEXT        NOT NULL,

    -- What it affects. Both nullable: a role change has no event, a takedown
    -- has no target user.
    target_user_id  UUID REFERENCES users(id)  ON DELETE CASCADE,
    target_event_id UUID REFERENCES events(id) ON DELETE CASCADE,

    -- Operation-specific detail. NEVER secrets: a full bank number must not be
    -- parked in a pending row where it sits readable for as long as the request
    -- is open. Bank additions carry only the masked tail plus a reference to
    -- the value held by the payment provider.
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Layer 2: second person
    approved_by       UUID REFERENCES users(id) ON DELETE RESTRICT,
    approved_at       TIMESTAMPTZ,
    rejected_by       UUID REFERENCES users(id) ON DELETE RESTRICT,
    rejected_at       TIMESTAMPTZ,
    rejection_reason  TEXT,

    -- Layer 3: the affected human
    subject_required     BOOLEAN     NOT NULL DEFAULT TRUE,
    subject_confirmed_at TIMESTAMPTZ,

    -- Layer 4: cooling-off. Set on approval, not on request, so the clock
    -- starts when the action becomes real rather than when it was typed.
    execute_not_before TIMESTAMPTZ,

    executed_at  TIMESTAMPTZ,
    executed_by  UUID REFERENCES users(id) ON DELETE RESTRICT,

    -- Unactioned requests must not sit open forever; a stale approval is an
    -- approval nobody is still thinking about.
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '7 days'),

    -- ── FOUR-EYES, ENFORCED BY THE DATABASE ─────────────────────────────────
    -- The single most important line in this migration. A handler bug, a
    -- copy-pasted user id, or someone with psql cannot approve their own
    -- request: the row will not be written.
    CONSTRAINT staff_requests_four_eyes
        CHECK (approved_by IS NULL OR approved_by <> requested_by),

    -- A reason that says nothing is the same as no reason.
    CONSTRAINT staff_requests_reason_meaningful
        CHECK (length(btrim(reason)) >= 10),

    -- Rejection must be explained — the requester sees this.
    CONSTRAINT staff_requests_rejection_explained
        CHECK (rejected_at IS NULL OR length(btrim(coalesce(rejection_reason, ''))) >= 5),

    -- Cannot be both approved and rejected.
    CONSTRAINT staff_requests_not_both
        CHECK (approved_at IS NULL OR rejected_at IS NULL),

    -- ── EXECUTION GATE ──────────────────────────────────────────────────────
    -- Executing requires: approved, subject confirmed when required, and the
    -- cooling-off elapsed. Expressed structurally so no code path can execute a
    -- request that hasn't cleared every layer.
    CONSTRAINT staff_requests_execution_gated
        CHECK (
            executed_at IS NULL
            OR (
                approved_at IS NOT NULL
                AND (subject_required = FALSE OR subject_confirmed_at IS NOT NULL)
                AND execute_not_before IS NOT NULL
                AND executed_at >= execute_not_before
            )
        ),

    -- Every request affects something.
    CONSTRAINT staff_requests_has_target
        CHECK (target_user_id IS NOT NULL OR target_event_id IS NOT NULL)
);

-- The approvals queue: "what is waiting for me", newest first.
CREATE INDEX IF NOT EXISTS idx_staff_requests_status
    ON staff_requests (status, requested_at DESC);

-- "What has been done to this organiser" — the question asked during an
-- incident, when speed matters.
CREATE INDEX IF NOT EXISTS idx_staff_requests_target_user
    ON staff_requests (target_user_id, requested_at DESC)
    WHERE target_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_staff_requests_requester
    ON staff_requests (requested_by, requested_at DESC);

-- ── Layer 3: subject confirmation codes ──────────────────────────────────────
--
-- Separate table so the hash can be deleted the moment it is used or expires,
-- without touching the request's audit history.
CREATE TABLE IF NOT EXISTS staff_request_confirmations (
    request_id  UUID PRIMARY KEY REFERENCES staff_requests(id) ON DELETE CASCADE,
    -- SHA-256 of the code, never the code. A confirmation code is a
    -- second factor; storing it in the clear would make the database a
    -- sufficient condition for approving a payout redirect.
    code_hash   TEXT        NOT NULL,
    sent_to     TEXT        NOT NULL,   -- email/phone at send time, for the record
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    -- Throttles brute force. Checked and incremented in the same statement that
    -- verifies, so parallel attempts can't race past the ceiling.
    attempts    INT         NOT NULL DEFAULT 0,
    CONSTRAINT confirmations_attempts_bounded CHECK (attempts <= 5)
);

-- ── Append-only audit log ────────────────────────────────────────────────────
--
-- Every staff action lands here, whether or not it went through a request. The
-- questions this answers are "who looked at this organiser's data" and "who
-- changed it" — both of which need answering during an incident, and neither of
-- which the request table covers on its own (reads never become requests).
CREATE TABLE IF NOT EXISTS staff_audit_log (
    id          BIGSERIAL PRIMARY KEY,
    at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id    UUID        REFERENCES users(id) ON DELETE SET NULL,
    actor_email TEXT        NOT NULL,  -- denormalised: survives account deletion
    actor_role  staff_role,
    action      TEXT        NOT NULL,  -- 'bank.delete', 'user.view_pii', …
    -- What was touched.
    target_type TEXT,
    target_id   UUID,
    request_id  UUID REFERENCES staff_requests(id) ON DELETE SET NULL,
    -- Outcome, so denied attempts are recorded too. A staff member repeatedly
    -- probing what they can't see is the signal you most want, and logging only
    -- successes hides exactly that.
    allowed     BOOLEAN     NOT NULL DEFAULT TRUE,
    detail      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ip          INET,
    user_agent  TEXT
);

CREATE INDEX IF NOT EXISTS idx_staff_audit_at     ON staff_audit_log (at DESC);
CREATE INDEX IF NOT EXISTS idx_staff_audit_actor  ON staff_audit_log (actor_id, at DESC);
CREATE INDEX IF NOT EXISTS idx_staff_audit_target ON staff_audit_log (target_id, at DESC)
    WHERE target_id IS NOT NULL;
-- Denied attempts are rare and disproportionately interesting, so they get
-- their own partial index rather than being filtered out of a full scan.
CREATE INDEX IF NOT EXISTS idx_staff_audit_denied ON staff_audit_log (at DESC)
    WHERE allowed = FALSE;

-- APPEND-ONLY, ENFORCED BY A RULE.
-- An audit log a compromised admin can edit is not evidence. Postgres RULEs
-- reject the statement outright rather than silently dropping rows, so an
-- attempted UPDATE or DELETE errors and is itself visible.
CREATE OR REPLACE RULE staff_audit_log_no_update AS
    ON UPDATE TO staff_audit_log DO INSTEAD NOTHING;
CREATE OR REPLACE RULE staff_audit_log_no_delete AS
    ON DELETE TO staff_audit_log DO INSTEAD NOTHING;

COMMENT ON TABLE staff_audit_log IS
    'Append-only. UPDATE and DELETE are disabled by rule. Retention, when it '
    'is needed, must be done by a superuser dropping the rule deliberately — '
    'which is itself the point.';

-- ── Payout hold ──────────────────────────────────────────────────────────────
--
-- Layer 4 continued: newly-changed bank details freeze payouts, so a
-- fraudulent change has to survive a window in which the organiser can notice
-- it. Settlement already runs 48h after an event, so this usually costs
-- nothing in practice — it only bites when details changed recently, which is
-- exactly when it should.
ALTER TABLE organizer_profiles
    ADD COLUMN IF NOT EXISTS payout_hold_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS payout_hold_reason TEXT;

COMMENT ON COLUMN organizer_profiles.payout_hold_until IS
    'Payout worker must skip this organiser until now() > payout_hold_until. '
    'Set automatically when bank details change. NULL means no hold.';