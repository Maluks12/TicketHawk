-- Reverses 000070.
--
--   DESTROYS THE DOCUMENT ACCESS LOG.
--
-- kyc_document_access_log records every read of an identity document and is
-- append-only by rule; dropping the table is the only way to remove it. If any
-- access has been recorded, dump it before running this — it is the evidence
-- required to answer "who looked at this person's ID", and it cannot be
-- reconstructed.
--
-- The pointer columns go too, so any row already exported loses its key and the
-- object in the external store becomes an orphan with nothing referencing it.
-- Recovering means re-importing from the store by matching organiser ids in the
-- object keys — possible, but manual.
--
-- organizers.id_document is untouched: this migration never moved the bytes, so
-- there is nothing to restore.

DROP RULE IF EXISTS kyc_access_log_no_update ON kyc_document_access_log;
DROP RULE IF EXISTS kyc_access_log_no_delete ON kyc_document_access_log;
DROP TABLE IF EXISTS kyc_document_access_log;

DROP INDEX IF EXISTS idx_organizers_pending_doc_export;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS id_document_key,
    DROP COLUMN IF EXISTS id_document_store,
    DROP COLUMN IF EXISTS id_document_sha256,
    DROP COLUMN IF EXISTS id_document_exported_at,
    DROP COLUMN IF EXISTS document_retain_until;