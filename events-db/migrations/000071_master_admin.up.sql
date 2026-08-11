-- ============================================================================
-- 000071: Master admin.
--
-- A SEPARATE FLAG, NOT A STAFF ROLE.
--
-- The obvious design is a fifth console_role above 'admin'. It is the wrong one,
-- because console roles are a LADDER and ladders accumulate: everyone who needs
-- to approve events day to day would also carry the ability to read every
-- organiser's identity document, since you cannot hold rung 4 without rung 5's
-- juniors.
--
-- These are different jobs at different frequencies:
--
--   console admin   operational, several people, used daily
--   master admin    break-glass, one or two people, used rarely
--
-- Keeping them orthogonal means granting someone console admin — which happens
-- as the team grows — can never accidentally grant them document access. The
-- two are set by different statements against different columns.
--
-- WHAT IT UNLOCKS
--
-- Reading an identity document from the external store (migration 000070), with
-- a mandatory reason, logged in kyc_document_access_log. Nothing else. A master
-- admin is not automatically a console admin and holds no console capability by
-- virtue of this flag.
-- ============================================================================

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_master_admin BOOLEAN NOT NULL DEFAULT FALSE,
    -- Who granted it and when. Not an audit log entry: this is a standing
    -- property of the account, and the question "who has this right now, and
    -- who gave it to them" should be answerable from the row itself rather than
    -- by replaying a log.
    ADD COLUMN IF NOT EXISTS master_admin_granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS master_admin_granted_at TIMESTAMPTZ;

COMMENT ON COLUMN users.is_master_admin IS
    'Break-glass right to read identity documents from the external store. '
    'Deliberately NOT part of console_role — see 000071 header. Grants no '
    'console capability on its own.';

-- Small set by design. The partial index makes "who holds this" a cheap
-- question, which matters because it should be audited periodically.
CREATE INDEX IF NOT EXISTS idx_users_master_admin
    ON users (id) WHERE is_master_admin = TRUE;