-- ============================================================================
-- 000094: Account restrictions — disable, ban and feature blocks.
--
-- WHAT THIS ADDS
--
-- A master admin can restrict any account that is not itself a master admin:
--
--   disable        locked out for a set number of days, then lifts itself
--   ban            locked out until a master admin lifts it
--   buy_tickets    ┐
--   create_events  │ one feature blocked, for a set number of days or
--   bank_details   │ indefinitely; the account signs in normally and can use
--   payouts        ┘ everything else
--
-- A locked-out account (disable or ban) is signed out everywhere: its refresh
-- tokens are revoked in the same transaction that writes the row, and
-- AuthMiddleware refuses its access tokens on the next request. A lockout also
-- holds the account's payouts. Live events stay on sale.
--
-- ONE ROW PER RESTRICTION, CHANGED ONLY TO LIFT IT
--
-- Nothing is ever edited in place. Lifting — by a master admin, or because a
-- newer restriction on the same thing replaced it — sets lifted_at, and a row
-- whose ends_at has passed simply stops being in force. The table is therefore
-- its own history: "what has been done to this account, by whom, and why" is a
-- single query.
--
-- "IN FORCE" IS DEFINED ONCE, IN A VIEW
--
-- active_account_restrictions is the only definition of a restriction that
-- applies right now: not lifted, and either indefinite or not yet ended. The
-- payout worker, the Paystack split and the API's in-memory registry all read
-- it, so a restriction can never be in force in one place and lapsed in
-- another.
--
-- WHY kind IS TEXT + CHECK, NOT AN ENUM
--
-- 000079 is the record of what enum labels cost here: ALTER TYPE ... ADD VALUE
-- cannot be used in the transaction that adds it, and a label the Go code
-- writes but the enum lacks surfaces as a 500. Adding a feature later means
-- replacing one CHECK constraint.
--
-- The same limits are enforced in Go (repository.ApplyAccountRestriction). The
-- constraints here are the backstop, not the primary check.
--
-- AUDIT
--
-- Every change is written to staff_audit_log ('user.restrict',
-- 'user.restriction_lift') inside the same transaction as the row.
-- ============================================================================

CREATE TABLE IF NOT EXISTS account_restrictions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    -- Internal. Shown to staff and written to the audit log, never to the
    -- account holder.
    reason       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    -- NULL = indefinite. A ban is always indefinite; a disable never is.
    ends_at      TIMESTAMPTZ,
    lifted_at    TIMESTAMPTZ,
    lifted_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    lift_reason  TEXT,

    CONSTRAINT account_restrictions_kind_valid CHECK (kind IN (
        'disable', 'ban', 'buy_tickets', 'create_events', 'bank_details', 'payouts'
    )),
    CONSTRAINT account_restrictions_reason_meaningful
        CHECK (length(btrim(reason)) BETWEEN 5 AND 500),
    CONSTRAINT account_restrictions_ban_indefinite
        CHECK (kind <> 'ban' OR ends_at IS NULL),
    CONSTRAINT account_restrictions_disable_ends
        CHECK (kind <> 'disable' OR ends_at IS NOT NULL),
    CONSTRAINT account_restrictions_ends_after_start
        CHECK (ends_at IS NULL OR ends_at > created_at),
    CONSTRAINT account_restrictions_lifted_after_start
        CHECK (lifted_at IS NULL OR lifted_at >= created_at)
);

-- Every enforcement path looks up rows that have not been lifted. Partial, so
-- lifted history never widens the hot path.
CREATE INDEX IF NOT EXISTS idx_account_restrictions_open
    ON account_restrictions (user_id, kind)
    WHERE lifted_at IS NULL;

-- One account's history, newest first — the console's detail panel.
CREATE INDEX IF NOT EXISTS idx_account_restrictions_user_history
    ON account_restrictions (user_id, created_at DESC);

CREATE OR REPLACE VIEW active_account_restrictions AS
    SELECT id, user_id, kind, reason, created_at, created_by, ends_at
    FROM account_restrictions
    WHERE lifted_at IS NULL
      AND (ends_at IS NULL OR ends_at > now());

COMMENT ON TABLE account_restrictions IS
    'Master-admin account restrictions: disable (N days), ban (indefinite) and '
    'feature blocks (buy_tickets, create_events, bank_details, payouts). Rows are '
    'never edited except to lift them. See migration 000094.';
COMMENT ON COLUMN account_restrictions.reason IS
    'Internal reason entered by the master admin. Never shown to the account holder.';
COMMENT ON COLUMN account_restrictions.ends_at IS
    'When the restriction stops applying. NULL means indefinite (always NULL for a ban).';
COMMENT ON VIEW active_account_restrictions IS
    'The single definition of a restriction in force: not lifted, and indefinite '
    'or not yet ended. Read by the payout worker, the Paystack split and the API.';