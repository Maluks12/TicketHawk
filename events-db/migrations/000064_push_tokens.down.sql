-- migrations/000064_push_tokens.down.sql
DROP INDEX IF EXISTS idx_push_tokens_user;
DROP TABLE IF EXISTS push_tokens;