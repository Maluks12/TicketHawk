-- migrations/000043_venue_type_max_tickets_per_user.up.sql
--
-- Adds two organiser-controlled fields to events:
--   venue_type           – overrides auto-detection in the mobile app ('stadium' | 'auditorium' | 'none')
--   max_tickets_per_user – caps tickets per purchase (NULL = no limit)

ALTER TABLE events
  ADD COLUMN IF NOT EXISTS venue_type           VARCHAR(20)
    CHECK (venue_type IN ('stadium', 'auditorium', 'none')),   -- NULL = auto-detect
  ADD COLUMN IF NOT EXISTS max_tickets_per_user INTEGER
    CHECK (max_tickets_per_user IS NULL OR (max_tickets_per_user >= 1 AND max_tickets_per_user <= 10));