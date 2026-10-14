-- ============================================================================
-- 000095: Case-insensitive email lookups.
--
-- Login, password reset and registration now match users.email on
-- LOWER(email) (repository.GetUserByEmail / EmailRegistered). The organiser
-- dashboard used to store addresses exactly as typed while the consumer apps
-- lowercase before login, so an organiser account created as "Jane@Gmail.com"
-- could not sign in on TicketHawk at all.
--
-- This migration ONLY adds the expression index that keeps those lookups an
-- index scan. It does not touch any user data. Lowercasing the stored
-- addresses is a separate, optional, manual step (see
-- normalise_user_emails.sql) so it can be checked before it runs.
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_users_email_lower ON users (LOWER(email));