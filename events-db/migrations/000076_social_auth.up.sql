-- ============================================================================
-- 000075: Sign in with Google and Sign in with Apple.
--
-- ── Why a separate table and not columns on users ───────────────────────────
--
-- The obvious shortcut is users.google_id / users.apple_id. It breaks the first
-- time somebody wants a second provider, and it makes "which accounts signed in
-- with Apple" a scan over a sparse column. More importantly it cannot express
-- the thing that actually matters here: one account, many proofs of identity.
-- A person can hold a password, a Google identity and an Apple identity
-- simultaneously and any of the three should get them into the SAME account.
--
-- So identities live in their own table, keyed on (provider, provider_subject).
--
-- ── Why the key is `subject` and not `email` ────────────────────────────────
--
-- Email is not a stable identifier at either provider. Google Workspace admins
-- rename accounts. Apple's Hide My Email issues a per-app relay address, and
-- a user can disable and re-enable it. The `sub` claim in the ID token is the
-- provider's permanent, per-app identifier for that person, and it is the ONLY
-- field either provider guarantees is stable. Matching on email would let a
-- reassigned address take over somebody else's account.
--
-- Email is still stored, but as a record of what the provider last told us —
-- never as a lookup key.
--
-- ── Why password becomes nullable ───────────────────────────────────────────
--
-- users.password is VARCHAR(255) NOT NULL, so a social-only account currently
-- cannot exist. The temptation is to write a placeholder string, and that is a
-- genuine security bug waiting to happen: every social account would share one
-- known bcrypt-invalid value, and any future code path that does a bcrypt
-- compare against it is one refactor away from treating them as equal. NULL is
-- unambiguous and cannot be compared against by accident — LoginHandler is
-- updated to reject a NULL password before it ever reaches bcrypt.
--
-- Note this does NOT weaken the existing check on password accounts: those
-- still get a bcrypt hash, and the app still requires one at registration.
--
-- ── On account deletion ─────────────────────────────────────────────────────
--
-- ON DELETE CASCADE on user_id is correct here and is NOT in tension with
-- 000065's soft-delete: identities are not financial records and nothing joins
-- through them. The soft-delete path deletes them explicitly (see
-- account_deletion_repository.go) so that a deleted account cannot be walked
-- back into via the provider — without that, signing in with Google again
-- would match the surviving identity row and resurrect the anonymised stub.
-- ============================================================================

-- ── Allow password-less accounts ────────────────────────────────────────────
ALTER TABLE users ALTER COLUMN password DROP NOT NULL;

-- phone_number and date_of_birth are already nullable in 000002, which is what
-- makes "collect at checkout" possible without further schema change.

CREATE TABLE user_identities (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- 'google' | 'apple'. A CHECK rather than an enum: adding a provider later
    -- is then a one-line constraint change instead of an ALTER TYPE, and this
    -- codebase has already been bitten once by an enum name collision (000050
    -- vs 000068's staff_role).
    provider         VARCHAR(32) NOT NULL CHECK (provider IN ('google', 'apple')),

    -- The provider's `sub` claim. Opaque, stable, per-app.
    provider_subject VARCHAR(255) NOT NULL,

    -- What the provider last told us. Informational only — never a lookup key.
    email            VARCHAR(255),
    email_verified   BOOLEAN     NOT NULL DEFAULT FALSE,

    -- True when Apple returned a @privaterelay.appleid.com address. Mail to it
    -- only routes while the app authorisation is alive, so ticket delivery has
    -- to treat these differently from a real address.
    is_private_relay BOOLEAN     NOT NULL DEFAULT FALSE,

    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_login_at    TIMESTAMP WITH TIME ZONE,

    -- The load-bearing constraint: one provider identity maps to exactly one
    -- account, forever. Without this a race between two concurrent first-time
    -- sign-ins would create two accounts for the same person.
    CONSTRAINT user_identities_provider_subject_key UNIQUE (provider, provider_subject)
);

CREATE INDEX idx_user_identities_user ON user_identities(user_id);

-- Supports "does this account already have Google linked" on the profile
-- screen without a sequential scan.
CREATE INDEX idx_user_identities_user_provider ON user_identities(user_id, provider);

-- ── Audit ───────────────────────────────────────────────────────────────────
-- Records HOW an account came into existence. Useful when a support ticket
-- says "I never signed up", and needed to decide whether prompting for a
-- password makes sense on the profile screen.
ALTER TABLE users ADD COLUMN IF NOT EXISTS signup_method VARCHAR(32) NOT NULL DEFAULT 'password';

COMMENT ON COLUMN users.signup_method IS
    'password | google | apple — how the account was originally created. Not a permission: an account created via google can later set a password and use both.';