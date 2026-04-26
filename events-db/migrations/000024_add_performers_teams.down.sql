-- migrations/000024_add_performers_teams.down.sql

ALTER TABLE events
    DROP COLUMN IF EXISTS performers,
    DROP COLUMN IF EXISTS teams;