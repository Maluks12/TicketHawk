DROP INDEX IF EXISTS idx_payouts_retry_due;

ALTER TABLE organizer_payouts
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS last_attempt_at,
    DROP COLUMN IF EXISTS attempts;