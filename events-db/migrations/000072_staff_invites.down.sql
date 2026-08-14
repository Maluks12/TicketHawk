-- Reverses 000072. Pending invites are lost; accepted ones already granted a
-- staff_role on users, which is untouched.

DROP INDEX IF EXISTS idx_staff_invites_status;
DROP INDEX IF EXISTS idx_staff_invites_open_email;
DROP TABLE IF EXISTS staff_invites;
DROP TYPE IF EXISTS staff_invite_status;