-- The hardened ResetPassword (single-use token burning) references
-- password_resets.used_at, but the table only ever had a `used` BOOLEAN
-- (migration 000002). Postgres therefore errored with "column used_at does
-- not exist" on EVERY reset attempt, which the handler surfaced as
-- "invalid or expired token" — password resets have been silently broken
-- since the auth hardening landed.

ALTER TABLE password_resets
    ADD COLUMN IF NOT EXISTS used_at TIMESTAMP WITH TIME ZONE;

-- Backfill: any token already burned under the old boolean scheme must not
-- become replayable within its expiry window after this migration.
UPDATE password_resets
SET    used_at = COALESCE(used_at, created_at)
WHERE  used = TRUE
AND    used_at IS NULL;