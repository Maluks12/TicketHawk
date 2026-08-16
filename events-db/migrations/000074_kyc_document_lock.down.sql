-- Reverses 000074.
--
-- Only the column is dropped. The staff_role backfill is NOT undone: rolling
-- back a schema change should not revoke console access that people have been
-- using since, and there is no way to tell an account backfilled by this
-- migration from one granted deliberately afterwards. Revoking staff_role is a
-- deliberate act through the console, not a side effect of a rollback.

DROP INDEX IF EXISTS idx_organizers_kyc_first_verified;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS kyc_first_verified_at;