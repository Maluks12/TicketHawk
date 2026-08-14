-- 000072: Staff invitations.
--
-- Two independent factors are required to complete an invite:
--   1. the emailed link token, which only the invitee receives
--   2. an OTP delivered to the MASTER ADMIN, who passes it on out of band
--
-- A compromised mailbox is therefore not enough to gain console access, and the
-- person granting access has to be actively involved at the moment it happens.

CREATE TYPE staff_invite_status AS ENUM (
    'pending',
    'started',
    'accepted',
    'revoked',
    'expired'
);

CREATE TABLE IF NOT EXISTS staff_invites (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email         TEXT        NOT NULL,
    staff_role    console_role NOT NULL,
    status        staff_invite_status NOT NULL DEFAULT 'pending',

    invited_by    UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    invited_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- SHA-256 of the link token and of the OTP. Neither is ever stored in the
    -- clear: the invite row must not be sufficient to accept the invite.
    token_hash    TEXT        NOT NULL UNIQUE,
    otp_hash      TEXT,
    otp_issued_at TIMESTAMPTZ,
    otp_attempts  INT         NOT NULL DEFAULT 0,

    accepted_at   TIMESTAMPTZ,
    accepted_user UUID REFERENCES users(id) ON DELETE SET NULL,
    revoked_at    TIMESTAMPTZ,

    expires_at    TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '7 days'),

    CONSTRAINT staff_invites_otp_attempts_bounded CHECK (otp_attempts <= 5)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_invites_open_email
    ON staff_invites (lower(email))
    WHERE status IN ('pending', 'started');

CREATE INDEX IF NOT EXISTS idx_staff_invites_status
    ON staff_invites (status, invited_at DESC);