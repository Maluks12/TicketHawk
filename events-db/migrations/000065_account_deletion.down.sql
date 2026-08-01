-- migrations/000065_account_deletion.down.sql
--
-- NOTE: rolling this back does NOT restore anonymised accounts. The original
-- email, name and phone number were destroyed in place at deletion time and are
-- not recoverable from anywhere in this database — which is the entire point of
-- the feature. Dropping the columns only removes the ability to TELL that those
-- rows are deleted, which would make anonymous stubs look like live accounts.
-- Do not run this on production without first exporting the affected ids.

DROP INDEX IF EXISTS idx_users_deleted_at;

ALTER TABLE users
    DROP COLUMN IF EXISTS deletion_reason,
    DROP COLUMN IF EXISTS deleted_at;