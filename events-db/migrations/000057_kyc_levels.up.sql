-- ── Tiered KYC ────────────────────────────────────────────────────────────────
-- Level 0: no verification — cannot create events (onboarding gate).
-- Level 1: BASIC  — legal name + SA ID number, validated locally (Luhn/DOB/age),
--                  no document, no VerifyNow call. Grants the organiser role
--                  and unlocks PRIVATE FREE events.
-- Level 2: FULL   — ID document + VerifyNow Home Affairs match (or admin
--                  approval). Unlocks PAID and PUBLIC events.
ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS kyc_level SMALLINT NOT NULL DEFAULT 0;

ALTER TABLE organizers
    ADD CONSTRAINT chk_kyc_level CHECK (kyc_level BETWEEN 0 AND 2);

-- Backfill: fully-verified organisers are level 2; anyone already holding the
-- organiser/admin role (pre-level accounts) gets at least level 1 so they are
-- not locked out of private free events they could already create.
UPDATE organizers SET kyc_level = 2 WHERE kyc_status = 'verified';
UPDATE organizers o
SET    kyc_level = 1
FROM   users u
WHERE  u.id = o.user_id
  AND  u.role IN ('organizer', 'admin')
  AND  o.kyc_level = 0;