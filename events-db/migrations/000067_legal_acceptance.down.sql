-- Reverses 000067.
--
-- Dropping these columns destroys the record of who agreed to what and when.
-- That record is the evidence the agreement was consented to, so take a dump
-- before running this if any acceptances have been collected.
--
-- Nothing else reads these columns: the gate fails open when the endpoint is
-- absent, so the dashboard keeps working with them gone.

DROP INDEX IF EXISTS idx_users_legal_version;

ALTER TABLE users
    DROP COLUMN IF EXISTS legal_accepted_version,
    DROP COLUMN IF EXISTS legal_accepted_at;