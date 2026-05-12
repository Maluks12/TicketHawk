-- migrations/000033_collection_cover_image.up.sql
--
-- Adds a dedicated cover image to merch_collections so organisers can
-- upload one canonical image per collection rather than relying on the
-- first product's images field to represent the whole collection in the UI.

ALTER TABLE merch_collections
    ADD COLUMN cover_image_url TEXT;