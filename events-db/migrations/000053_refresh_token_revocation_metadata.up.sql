-- Reuse detection currently treats EVERY presentation of a revoked refresh
-- token as theft and revokes all the user's sessions. But the most common
-- cause is innocent: two tabs (or a slow request) racing to refresh — the
-- winner rotates the token, the loser presents the just-rotated one, and the
-- user gets logged out of everything. To tell an innocent race (seconds after
-- a rotation) apart from a genuine replay (long after / after a logout or
-- password reset), we need to know WHEN and WHY each token was revoked.

ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS revoked_at     TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS revoked_reason TEXT;

-- Backfill existing revoked rows with their created_at (old, so none of them
-- can qualify for the new rotation-grace window) and a 'legacy' reason.
UPDATE refresh_tokens
SET    revoked_at     = COALESCE(revoked_at, created_at),
       revoked_reason = COALESCE(revoked_reason, 'legacy')
WHERE  revoked = TRUE;