-- migrations/000001_create_enums_and_extensions.down.sql

DROP TYPE IF EXISTS organizer_status;
DROP TYPE IF EXISTS payment_method_type;
DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS booking_status;
DROP TYPE IF EXISTS event_status;
DROP TYPE IF EXISTS user_role;

DROP EXTENSION IF EXISTS "pg_trgm";
DROP EXTENSION IF EXISTS "uuid-ossp";