-- ============================================================================
-- 000070: Move KYC identity documents out of the operational database.
--
-- WHERE THEY ARE NOW
--
-- organizers.id_document is a BYTEA — the raw image sits in the same table the
-- admin console reads on every page load, in the same database as bookings and
-- payouts, in every nightly RDS snapshot, and in any dump anyone takes to debug
-- something unrelated. That is the highest-sensitivity data in the product
-- living in the most widely-touched place.
--
-- WHERE THEY GO
--
-- A separate store (S3 bucket with its own KMS key and its own access policy),
-- with only a POINTER kept here. The identity FIELDS stay in this database —
-- the SA ID number, the name, the DHA result — because those are what the
-- regulatory record consists of and what the console needs to verify. The
-- IMAGE moves, because nothing in the day-to-day workflow needs to see it.
--
-- WHAT THIS BUYS
--
--   • A database dump no longer contains anyone's ID photograph.
--   • Reading a document requires credentials for a different system, so it
--     cannot happen as a side effect of an ordinary console query.
--   • Access to the images is logged by the object store independently of this
--     application, which means the log survives a compromise of this one.
--
-- RETENTION
--
-- FICA requires identity records be kept for 5 years after the business
-- relationship ends. document_retain_until carries that date so the store can
-- expire objects on a schedule rather than keeping them forever by default —
-- indefinite retention of ID photographs is its own POPIA problem.
--
-- ⚠️  THIS MIGRATION DOES NOT MOVE THE BYTES. It adds the pointer columns and
-- leaves id_document in place. Dropping it is a separate, deliberate step AFTER
-- the export has run and been verified — see MIGRATE_KYC_DOCS.md.
-- ============================================================================

ALTER TABLE organizers
    -- Object key in the external store, e.g. 'kyc/2026/08/<uuid>.jpg'. NULL
    -- means "not yet exported" — which is every existing row until the backfill
    -- runs, and is how the exporter finds its work.
    ADD COLUMN IF NOT EXISTS id_document_key      TEXT,
    -- Which store the key belongs to. Recorded per row rather than assumed from
    -- config so a future bucket migration can proceed row by row instead of
    -- flipping a global switch and orphaning everything already written.
    ADD COLUMN IF NOT EXISTS id_document_store    TEXT,
    -- SHA-256 of the bytes at export time. Lets a later read prove the object
    -- it fetched is the one that was verified, and detects silent corruption or
    -- substitution in the external store.
    ADD COLUMN IF NOT EXISTS id_document_sha256   TEXT,
    ADD COLUMN IF NOT EXISTS id_document_exported_at TIMESTAMPTZ,
    -- FICA retention horizon. Set at export; the store expires on it.
    ADD COLUMN IF NOT EXISTS document_retain_until   TIMESTAMPTZ;

COMMENT ON COLUMN organizers.id_document_key IS
    'Object key in the external KYC document store. NULL = not yet exported. '
    'The image itself must never be written back into this database.';

COMMENT ON COLUMN organizers.id_document IS
    'DEPRECATED — raw image bytes. Being migrated to the external store; see '
    'id_document_key. Do not add new reads. This column is dropped in a later '
    'migration once the export is verified.';

-- The exporter's work queue: rows with bytes but no key.
CREATE INDEX IF NOT EXISTS idx_organizers_pending_doc_export
    ON organizers (id)
    WHERE id_document IS NOT NULL AND id_document_key IS NULL;

-- ── Document access log ──────────────────────────────────────────────────────
--
-- Separate from staff_audit_log on purpose. That table records what happened in
-- the console; this records every access to an identity document, including
-- ones that did not originate from the console at all (a break-glass read, a
-- regulator request, an export job).
--
-- Append-only by the same rules as staff_audit_log: a log of who looked at ID
-- documents is worthless if the people who can look at them can also edit it.
CREATE TABLE IF NOT EXISTS kyc_document_access_log (
    id           BIGSERIAL PRIMARY KEY,
    at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    organizer_id UUID        REFERENCES organizers(id) ON DELETE SET NULL,
    -- Denormalised: the record of who read a document must outlive the reader's
    -- account, or deleting an account erases the evidence.
    actor_id     UUID,
    actor_email  TEXT        NOT NULL,
    -- 'view', 'export', 'regulator_request', 'retention_delete'
    action       TEXT        NOT NULL,
    -- Required, and enforced. A read of an identity document without a stated
    -- reason is exactly the read you would want to ask about later.
    reason       TEXT        NOT NULL,
    document_key TEXT,
    ip           INET,
    user_agent   TEXT,
    CONSTRAINT kyc_access_reason_meaningful CHECK (length(btrim(reason)) >= 10)
);

CREATE INDEX IF NOT EXISTS idx_kyc_access_at        ON kyc_document_access_log (at DESC);
CREATE INDEX IF NOT EXISTS idx_kyc_access_organizer ON kyc_document_access_log (organizer_id, at DESC);

CREATE OR REPLACE RULE kyc_access_log_no_update AS
    ON UPDATE TO kyc_document_access_log DO INSTEAD NOTHING;
CREATE OR REPLACE RULE kyc_access_log_no_delete AS
    ON DELETE TO kyc_document_access_log DO INSTEAD NOTHING;

COMMENT ON TABLE kyc_document_access_log IS
    'Every access to an identity document, from any path. Append-only: UPDATE '
    'and DELETE are disabled by rule.';