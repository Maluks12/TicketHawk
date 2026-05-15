-- Reverse migration 000036
-- Note: PostgreSQL cannot remove enum values once added.
-- Drop the merch enum type only (event_status values cannot be removed).
DROP TYPE IF EXISTS merch_approval_status;