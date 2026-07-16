ALTER TABLE refresh_tokens
    DROP COLUMN IF EXISTS revoked_at,
    DROP COLUMN IF EXISTS revoked_reason;