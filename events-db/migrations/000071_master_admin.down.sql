-- Reverses 000071.
--
-- Removes the master-admin flag from every account. Nobody can read identity
-- documents afterwards — which is a safe direction to fail, but note it also
-- removes the record of WHO held the right and who granted it, so an audit of
-- past access becomes harder to interpret.
--
-- kyc_document_access_log (000070) is untouched, so the record of what was
-- actually read survives.

DROP INDEX IF EXISTS idx_users_master_admin;

ALTER TABLE users
    DROP COLUMN IF EXISTS is_master_admin,
    DROP COLUMN IF EXISTS master_admin_granted_by,
    DROP COLUMN IF EXISTS master_admin_granted_at;