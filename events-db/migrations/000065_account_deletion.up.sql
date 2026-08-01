-- migrations/000065_account_deletion.up.sql
--
-- In-app account deletion (Apple App Store Guideline 5.1.1(v)).
--
-- ── Why soft-delete and not DELETE FROM users ────────────────────────────────
--
-- It is not a preference, it is a hard constraint. Two foreign keys use
-- ON DELETE RESTRICT:
--
--     bookings.user_id      -> users(id) ON DELETE RESTRICT   (000005)
--     events.organizer_id   -> users(id) ON DELETE RESTRICT   (000004)
--
-- so a hard delete of anyone who has ever bought a ticket fails at the database
-- and surfaces as a 500. Even if those were CASCADE, cascading them would be
-- wrong: bookings and payments are financial records. The CPA and SARS both
-- expect a seller to be able to produce the transaction history behind a sale
-- for years after it happened, and organiser payouts, refunds and clawbacks all
-- join through booking rows. Deleting a buyer would silently corrupt an
-- organiser's settlement history.
--
-- What the user actually has a right to is that their PERSONAL DATA stops being
-- personal data. So the account row survives as an anonymous stub that the
-- financial records can still point at, and every identifying field on it is
-- destroyed in place. After deletion there is no email, no name, no phone
-- number and no usable password anywhere in the row — nothing that ties the
-- remaining transaction records back to a person.
--
-- ── On the email column ──────────────────────────────────────────────────────
--
-- users.email is UNIQUE and NOT NULL, so it cannot simply be nulled. It is
-- overwritten with a per-account value derived from the (already random) UUID
-- primary key at deletion time:
--
--     deleted-<uuid>@deleted.invalid
--
-- `.invalid` is reserved by RFC 2606 and can never be registered, so the
-- address is guaranteed undeliverable and un-ownable. This also means the
-- original address is immediately free for the person to sign up with again.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS deleted_at       TIMESTAMP WITH TIME ZONE,
    -- Free-text, optional, supplied by the user on the confirm screen. Product
    -- feedback only; never anything they typed about themselves.
    ADD COLUMN IF NOT EXISTS deletion_reason  TEXT;

-- Every authentication path filters on `deleted_at IS NULL`, and every one of
-- them looks a user up by email or by id. Partial index so the cost falls only
-- on the (small) deleted set rather than widening the hot path.
CREATE INDEX IF NOT EXISTS idx_users_deleted_at
    ON users (deleted_at)
    WHERE deleted_at IS NOT NULL;

COMMENT ON COLUMN users.deleted_at IS
    'Set when the user deletes their own account. The row is retained ONLY as an '
    'anonymous stub so that bookings/payments FKs (ON DELETE RESTRICT) stay '
    'valid; all identifying fields are destroyed at the same time. A non-null '
    'value must be treated as "this account does not exist" by every auth path.';