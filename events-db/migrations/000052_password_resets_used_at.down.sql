-- Preserve burned-token state back onto the legacy boolean before dropping.
UPDATE password_resets
SET    used = TRUE
WHERE  used_at IS NOT NULL;

ALTER TABLE password_resets
    DROP COLUMN IF EXISTS used_at;