-- migrations/000003_create_categories_and_organizers.down.sql

DROP TRIGGER IF EXISTS update_organizers_updated_at ON organizers;
DROP TABLE IF EXISTS organizers CASCADE;
DROP TABLE IF EXISTS categories CASCADE;