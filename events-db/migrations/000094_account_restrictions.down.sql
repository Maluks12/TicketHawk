-- Rolling back lifts every restriction at once. Sessions that were ended by a
-- disable or ban stay ended; those people simply sign in again.
DROP VIEW IF EXISTS active_account_restrictions;
DROP TABLE IF EXISTS account_restrictions;