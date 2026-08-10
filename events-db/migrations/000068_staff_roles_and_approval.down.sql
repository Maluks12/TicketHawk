-- Reverses 000068.
--
-- WARNING: this destroys the audit log.
--
-- staff_audit_log is deliberately append-only (UPDATE/DELETE disabled by rule),
-- and dropping the table is the one way to remove it. If any staff activity has
-- been recorded, dump it before running this — it is the evidence of who
-- accessed and changed organiser data, and it cannot be reconstructed.
--
-- The rules must go first: DROP TABLE succeeds regardless, but dropping them
-- explicitly makes the destruction of the protection a visible line in the
-- migration rather than a side effect.

DROP RULE IF EXISTS staff_audit_log_no_update ON staff_audit_log;
DROP RULE IF EXISTS staff_audit_log_no_delete ON staff_audit_log;

DROP TABLE IF EXISTS staff_audit_log;
DROP TABLE IF EXISTS staff_request_confirmations;
DROP TABLE IF EXISTS staff_requests;

DROP TYPE IF EXISTS staff_request_status;
DROP TYPE IF EXISTS staff_request_kind;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS payout_hold_until,
    DROP COLUMN IF EXISTS payout_hold_reason;

-- Dropped last: the enum can't go while a column still uses it.
DROP INDEX IF EXISTS idx_users_staff_role;
ALTER TABLE users DROP COLUMN IF EXISTS staff_role;
DROP TYPE IF EXISTS staff_role;