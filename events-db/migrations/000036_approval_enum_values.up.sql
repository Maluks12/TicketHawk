-- ============================================================
-- Migration 000036: Add new enum values for event and merch approval
--
-- IMPORTANT: This must be a separate migration from 000037.
-- PostgreSQL does not allow a newly added enum value to be used
-- in the same transaction where it was created. golang-migrate
-- wraps each file in a transaction, so enum additions and their
-- usage must be in separate files.
-- ============================================================

-- 1. Extend event_status enum
ALTER TYPE event_status ADD VALUE IF NOT EXISTS 'pending_review';
ALTER TYPE event_status ADD VALUE IF NOT EXISTS 'rejected';

-- 2. Create merch_approval_status enum
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'merch_approval_status') THEN
        CREATE TYPE merch_approval_status AS ENUM (
            'pending_review',
            'approved',
            'rejected'
        );
    END IF;
END
$$;