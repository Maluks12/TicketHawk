-- migrations/000033_collection_cover_image.down.sql

ALTER TABLE merch_collections
    DROP COLUMN IF EXISTS cover_image_url;