-- Reverses 000073. Active grants are lost; nobody but master admins can view
-- identity documents afterwards, which is a safe direction to fail.
-- kyc_document_access_log is untouched, so the record of past reads survives.

DROP INDEX IF EXISTS idx_kyc_grants_pending;
DROP INDEX IF EXISTS idx_kyc_grants_active;
DROP TABLE IF EXISTS kyc_document_grants;