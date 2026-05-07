-- migrations/000029_merch_collections.down.sql

ALTER TABLE merchandise DROP COLUMN IF EXISTS collection_id;
DROP TABLE IF EXISTS merch_collections;