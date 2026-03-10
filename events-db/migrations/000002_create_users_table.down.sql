-- migrations/000002_create_users_table.down.sql

DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS password_resets CASCADE;
DROP TABLE IF EXISTS users CASCADE;