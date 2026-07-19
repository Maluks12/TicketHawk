-- migrations/000055_kyc_and_visibility.up.sql
--
-- KYC & event-visibility layer (design doc sections 3, 4, 6).
--
--   Event visibility:
--     'public'       — discoverable; publish path stays draft → pending_review
--                      → published (existing admin approval satisfies the
--                      doc's "manual review" requirement for public events).
--     'private_link' — reachable only by slug/link. Publishes to the NEW
--                      status 'published_private', which every discovery
--                      query (feed/search/home filter on status='published')
--                      automatically excludes with zero query changes. A
--                      missed functional check fails safe: the event can't
--                      be booked, rather than leaking into the public feed.
--
--   Organizer KYC:
--     Manual path: SA ID checksum + ID photo + admin review.
--     Didit path : automatic DHA registry check; full_match auto-verifies.
--     The 13-digit ID number is stored AES-GCM encrypted; only the last 4
--     digits are stored in the clear. ID documents live in the database as
--     BYTEA — unambiguously private (the R2 bucket is public via CDN), tiny
--     at this scale, and served only through an authenticated admin endpoint.
--
--   Grandfathering: no backfill. Existing organizers stay kyc_status='none'
--   and existing published events are untouched; the matrix is enforced only
--   inside PublishEvent, so it bites when an organizer next publishes a paid
--   event. Admin-created events (Path 4) bypass PublishEvent entirely and
--   remain KYC-exempt.

-- New event status for link-only events. Added via ::text-safe comparisons
-- below so this file stays single-transaction compatible.
ALTER TYPE event_status ADD VALUE IF NOT EXISTS 'published_private';

-- ── Events: visibility + rights declaration ──────────────────────────────────

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS visibility VARCHAR(15) NOT NULL DEFAULT 'public',
    ADD COLUMN IF NOT EXISTS rights_declared_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS rights_declaration_version INT;

ALTER TABLE events
    ADD CONSTRAINT chk_event_visibility CHECK (visibility IN ('public', 'private_link'));

-- A public event can never sit in published_private and vice versa.
-- (::text comparisons — the enum label added above can't be referenced as an
-- enum literal inside the same transaction.)
ALTER TABLE events
    ADD CONSTRAINT chk_visibility_status_consistency CHECK (
        NOT (status::text = 'published'         AND visibility = 'private_link') AND
        NOT (status::text = 'published_private' AND visibility = 'public')
    );

-- ── Users: phone-verification ready (no OTP in v1; schema prepared) ──────────

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS phone_verified_at TIMESTAMP WITH TIME ZONE;

-- ── Organizers: KYC ──────────────────────────────────────────────────────────

ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS legal_first_name       VARCHAR(100),
    ADD COLUMN IF NOT EXISTS legal_last_name        VARCHAR(100),
    ADD COLUMN IF NOT EXISTS sa_id_encrypted        BYTEA,        -- AES-GCM(13-digit ID)
    ADD COLUMN IF NOT EXISTS sa_id_last4            VARCHAR(4),
    ADD COLUMN IF NOT EXISTS sa_id_dob              DATE,         -- derived from ID; needed for registry checks
    ADD COLUMN IF NOT EXISTS kyc_status             VARCHAR(15) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS kyc_method             VARCHAR(10),  -- 'manual' | 'didit'
    ADD COLUMN IF NOT EXISTS kyc_submitted_at       TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS kyc_verified_at        TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS kyc_reviewed_by        UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS kyc_rejection_reason   TEXT,
    ADD COLUMN IF NOT EXISTS id_document            BYTEA,
    ADD COLUMN IF NOT EXISTS id_document_mime       VARCHAR(50),
    ADD COLUMN IF NOT EXISTS proof_of_address       BYTEA,
    ADD COLUMN IF NOT EXISTS proof_of_address_mime  VARCHAR(50),
    ADD COLUMN IF NOT EXISTS registry_check         JSONB,        -- raw Didit response, audit trail
    ADD COLUMN IF NOT EXISTS registry_checked_at    TIMESTAMP WITH TIME ZONE;

ALTER TABLE organizers
    ADD CONSTRAINT chk_kyc_status
    CHECK (kyc_status IN ('none', 'pending_review', 'verified', 'rejected'));

-- Verified requires an actual identity on file.
ALTER TABLE organizers
    ADD CONSTRAINT chk_verified_requires_identity
    CHECK (kyc_status <> 'verified' OR (sa_id_encrypted IS NOT NULL AND legal_last_name IS NOT NULL));

CREATE INDEX IF NOT EXISTS idx_organizers_kyc_status
    ON organizers(kyc_status) WHERE kyc_status = 'pending_review';