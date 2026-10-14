-- ============================================================================
-- normalise_user_emails.sql — OPTIONAL, run by hand, one step at a time.
--
-- The login fix (case-insensitive lookup) works WITHOUT this script. This just
-- tidies legacy rows that were stored with capitals by the organiser signup,
-- so every stored address is in the same canonical lowercase form.
--
-- Run each STEP separately and read the output before moving on.
-- ============================================================================

-- STEP 1 — How many accounts are stored with capitals?
SELECT id, email, created_at
FROM users
WHERE email <> LOWER(email)
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- STEP 2 — Any addresses that exist twice, differing only by case?
-- These are NOT changed by step 3. If this returns rows, decide per pair which
-- account is the real one before doing anything else.
SELECT LOWER(email) AS email_lower, COUNT(*) AS accounts,
       ARRAY_AGG(id ORDER BY created_at) AS user_ids,
       ARRAY_AGG(email ORDER BY created_at) AS stored_as
FROM users
GROUP BY LOWER(email)
HAVING COUNT(*) > 1;

-- STEP 3 — Lowercase every address that does NOT collide with another row.
-- Wrapped in a transaction: check the UPDATE count matches step 1 (minus any
-- step-2 collisions), then COMMIT. Anything unexpected → ROLLBACK.
BEGIN;

UPDATE users u
SET email = LOWER(u.email)
WHERE u.email <> LOWER(u.email)
  AND NOT EXISTS (
    SELECT 1 FROM users o
    WHERE o.id <> u.id
      AND LOWER(o.email) = LOWER(u.email)
  );

-- COMMIT;
-- ROLLBACK;