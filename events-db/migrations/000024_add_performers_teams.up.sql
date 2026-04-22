-- migrations/000024_add_performers_teams.up.sql

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS performers TEXT[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS teams      TEXT[] DEFAULT '{}';

