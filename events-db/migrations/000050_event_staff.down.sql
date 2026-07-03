ALTER TABLE ticket_assignments DROP COLUMN IF EXISTS scanned_by_user_id;
DROP TABLE IF EXISTS event_staff;
DROP TABLE IF EXISTS event_staff_codes;
DROP TYPE IF EXISTS staff_status;
DROP TYPE IF EXISTS staff_role;