-- Reverse 000085.
--
-- Lossy: dropping rfid_tags discards the tag estate and its lifecycle audit.
-- That is acceptable only because Phase 1 tags carry no money — re-running the
-- up-migration and re-registering from the encoder's own batch export restores
-- the state. Once Phase 3 lands, this down-migration must be revisited: a
-- wallet ledger referencing tags cannot be dropped this casually.
--
-- Order matters. scan_events columns go first (they are the only thing outside
-- this migration's own objects), then the tables in dependency order, then the
-- types — a type cannot be dropped while any column still uses it.

DROP INDEX IF EXISTS idx_scan_events_tag;

ALTER TABLE scan_events
    DROP COLUMN IF EXISTS medium,
    DROP COLUMN IF EXISTS tag_uid;

DROP TYPE IF EXISTS scan_medium;

DROP TRIGGER IF EXISTS trg_rfid_bump_batch_activated ON rfid_tags;
DROP FUNCTION IF EXISTS rfid_bump_batch_activated();

DROP TABLE IF EXISTS rfid_tag_events;
DROP TYPE  IF EXISTS tag_event_kind;

DROP TABLE IF EXISTS rfid_tags;
DROP TABLE IF EXISTS rfid_tag_batches;

DROP TYPE IF EXISTS tag_status;
DROP TYPE IF EXISTS tag_verification_class;
DROP TYPE IF EXISTS tag_chip_type;