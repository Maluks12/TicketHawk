-- ============================================================================
-- 000067: Record which version of the organiser agreements each account
--         accepted, and when.
--
-- WHY A VERSION AND NOT A BOOLEAN
--
-- `terms_accepted BOOLEAN` answers "did they ever tick a box", which is not the
-- question that matters. The question is "did they accept THE TERMS CURRENTLY
-- IN FORCE" — and the moment the commission split or the refund policy changes,
-- every existing true becomes a claim about a document that no longer exists.
--
-- Storing the version means a material revision is a one-line constant bump in
-- lib/legalLinks.ts (CURRENT_LEGAL_VERSION), after which every organiser is
-- re-prompted exactly once. Nothing is migrated, nothing is reset, and the
-- audit trail still shows precisely which revision each person agreed to.
--
-- WHY ON users AND NOT organizer_profiles
--
-- Acceptance is by the legal person, not by the organiser role. Someone can
-- lose and regain the organiser role (KYC lapse, re-application) and their
-- agreement to the terms is unaffected by that. Attaching it to the profile
-- would also mean a pending applicant — who reaches /onboarding and /kyc before
-- the role is granted — has nowhere to record consent.
--
-- Both columns are NULLABLE with no default, and that is the load-bearing part:
-- every existing account starts NULL, which the gate reads as "has not accepted
-- the current version" and prompts. Defaulting to the current version would
-- silently record an agreement nobody made.
-- ============================================================================

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS legal_accepted_version TEXT,
    ADD COLUMN IF NOT EXISTS legal_accepted_at      TIMESTAMPTZ;

COMMENT ON COLUMN users.legal_accepted_version IS
    'Version string of the agreement set the user accepted, e.g. 2026-08-01. '
    'NULL means never accepted. Compared against CURRENT_LEGAL_VERSION in the '
    'client; anything older re-prompts.';

COMMENT ON COLUMN users.legal_accepted_at IS
    'Server clock at the moment of acceptance. Server-set, never client-set — '
    'a client-supplied timestamp is an attacker-supplied timestamp, and this '
    'column is the evidence that consent was given.';

-- Partial index: the only query is "has this user accepted the current
-- version", and rows that have never accepted are the ones being looked up.
CREATE INDEX IF NOT EXISTS idx_users_legal_version
    ON users (legal_accepted_version)
    WHERE legal_accepted_version IS NOT NULL;