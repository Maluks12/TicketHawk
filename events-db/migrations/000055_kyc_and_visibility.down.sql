-- migrations/000055_kyc_and_visibility.down.sql
-- Note: PostgreSQL cannot drop an enum VALUE. 'published_private' stays on the
-- type (harmless — nothing references it after this down runs; any events in
-- that status are folded back to draft so no rows use the label).

UPDATE events SET status = 'draft' WHERE status::text = 'published_private';

ALTER TABLE events
    DROP CONSTRAINT IF EXISTS chk_visibility_status_consistency,
    DROP CONSTRAINT IF EXISTS chk_event_visibility;

ALTER TABLE events
    DROP COLUMN IF EXISTS visibility,
    DROP COLUMN IF EXISTS rights_declared_at,
    DROP COLUMN IF EXISTS rights_declaration_version;

ALTER TABLE users
    DROP COLUMN IF EXISTS phone_verified_at;

ALTER TABLE organizers
    DROP CONSTRAINT IF EXISTS chk_verified_requires_identity,
    DROP CONSTRAINT IF EXISTS chk_kyc_status;

DROP INDEX IF EXISTS idx_organizers_kyc_status;

ALTER TABLE organizers
    DROP COLUMN IF EXISTS legal_first_name,
    DROP COLUMN IF EXISTS legal_last_name,
    DROP COLUMN IF EXISTS sa_id_encrypted,
    DROP COLUMN IF EXISTS sa_id_last4,
    DROP COLUMN IF EXISTS sa_id_dob,
    DROP COLUMN IF EXISTS kyc_status,
    DROP COLUMN IF EXISTS kyc_method,
    DROP COLUMN IF EXISTS kyc_submitted_at,
    DROP COLUMN IF EXISTS kyc_verified_at,
    DROP COLUMN IF EXISTS kyc_reviewed_by,
    DROP COLUMN IF EXISTS kyc_rejection_reason,
    DROP COLUMN IF EXISTS id_document,
    DROP COLUMN IF EXISTS id_document_mime,
    DROP COLUMN IF EXISTS proof_of_address,
    DROP COLUMN IF EXISTS proof_of_address_mime,
    DROP COLUMN IF EXISTS registry_check,
    DROP COLUMN IF EXISTS registry_checked_at;