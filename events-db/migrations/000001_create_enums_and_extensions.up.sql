-- migrations/000001_create_enums_and_extensions.up.sql

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- For text search

-- Create ENUM types
CREATE TYPE user_role AS ENUM ('attendee', 'organizer', 'admin');
CREATE TYPE event_status AS ENUM ('draft', 'published', 'cancelled', 'completed');
CREATE TYPE booking_status AS ENUM ('pending', 'confirmed', 'cancelled', 'refunded');
CREATE TYPE payment_status AS ENUM ('pending', 'processing', 'completed', 'failed', 'refunded');
CREATE TYPE payment_method_type AS ENUM ('credit_card', 'paypal', 'apple_pay', 'google_pay', 'samsung_pay');
CREATE TYPE organizer_status AS ENUM ('pending', 'approved', 'suspended', 'rejected');