-- 000073: Time-boxed grants to view an organiser's ID document.
--
-- Admin and editor cannot read identity documents on their own. They ask, a
-- master admin approves, and the resulting grant is scoped to one organiser and
-- expires. It is not a capability, so promoting someone to admin never confers
-- it and revoking the grant does not require changing their role.

CREATE TABLE IF NOT EXISTS kyc_document_grants (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organizer_id  UUID NOT NULL REFERENCES organizers(id) ON DELETE CASCADE,
    granted_to    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    requested_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason        TEXT        NOT NULL,

    approved_by   UUID REFERENCES users(id) ON DELETE RESTRICT,
    approved_at   TIMESTAMPTZ,
    denied_by     UUID REFERENCES users(id) ON DELETE RESTRICT,
    denied_at     TIMESTAMPTZ,

    expires_at    TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ,

    CONSTRAINT kyc_grant_reason_meaningful CHECK (length(btrim(reason)) >= 10),
    CONSTRAINT kyc_grant_not_both CHECK (approved_at IS NULL OR denied_at IS NULL),
    -- A master admin cannot approve their own request; they do not need one.
    CONSTRAINT kyc_grant_four_eyes CHECK (approved_by IS NULL OR approved_by <> granted_to),
    CONSTRAINT kyc_grant_expiry_set CHECK (approved_at IS NULL OR expires_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_kyc_grants_active
    ON kyc_document_grants (granted_to, organizer_id, expires_at)
    WHERE approved_at IS NOT NULL AND revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_kyc_grants_pending
    ON kyc_document_grants (requested_at DESC)
    WHERE approved_at IS NULL AND denied_at IS NULL AND revoked_at IS NULL;