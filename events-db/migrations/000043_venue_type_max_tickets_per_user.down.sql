-- migrations/000043_venue_type_max_tickets_per_user.down.sql

ALTER TABLE events
  DROP COLUMN IF EXISTS venue_type,
  DROP COLUMN IF EXISTS max_tickets_per_user;