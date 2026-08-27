-- Rollback for 000075.
--
-- WARNING: restoring the NOT NULL on users.password will FAIL if any
-- social-only account exists (they have password IS NULL by design). That is
-- deliberate — silently inventing a password for those rows would leave
-- unlogin-able accounts behind with no signal. If you genuinely need to roll
-- back after social accounts exist, decide explicitly what should happen to
-- them first: either delete them, or send each a password-reset link and let
-- them set one.

DROP INDEX IF EXISTS idx_user_identities_user_provider;
DROP INDEX IF EXISTS idx_user_identities_user;
DROP TABLE IF EXISTS user_identities;

ALTER TABLE users DROP COLUMN IF EXISTS signup_method;

ALTER TABLE users ALTER COLUMN password SET NOT NULL;