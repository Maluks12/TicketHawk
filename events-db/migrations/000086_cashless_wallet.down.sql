-- Reverse 000086.
--
-- READ THIS BEFORE RUNNING IT IN PRODUCTION.
--
-- Unlike 000085, this migration is destructive in a way that matters: dropping
-- cashless_transactions destroys the only record of money taken from and spent
-- by attendees. There is no encoder export to re-import from, and an attendee
-- disputing a charge afterwards cannot be answered.
--
-- Run it freely on a scratch database — testing the down-migration now, while
-- nothing depends on these tables, is exactly the right time. Once a single
-- real top-up has settled, treat this file as a development tool and take a
-- dump first.
--
-- Order: the rfid_tags additions go first (they are the only objects outside
-- this migration), then tables in dependency order, then the types.

DROP INDEX IF EXISTS uq_rfid_active_wallet;
DROP INDEX IF EXISTS idx_rfid_tags_wallet;

ALTER TABLE rfid_tags DROP CONSTRAINT IF EXISTS chk_rfid_wallet_requires_verifiable_chip;
ALTER TABLE rfid_tags DROP COLUMN IF EXISTS wallet_id;

DROP TRIGGER IF EXISTS trg_cashless_touch_funding ON cashless_topups;
DROP FUNCTION IF EXISTS cashless_touch_funding();

DROP TABLE IF EXISTS wallet_balance_snapshots;
DROP TABLE IF EXISTS cashless_topups;
DROP TYPE  IF EXISTS cashless_topup_status;
DROP TYPE  IF EXISTS cashless_topup_method;

DROP TABLE IF EXISTS cashless_transactions;
DROP TYPE  IF EXISTS cashless_txn_mode;
DROP TYPE  IF EXISTS cashless_txn_kind;

DROP TABLE IF EXISTS cashless_wallets;
DROP TYPE  IF EXISTS wallet_funding;
DROP TYPE  IF EXISTS cashless_wallet_status;

DROP TABLE IF EXISTS vendor_pos_devices;
DROP TYPE  IF EXISTS pos_device_status;

DROP TABLE IF EXISTS vendor_products;
DROP TABLE IF EXISTS event_vendors;
DROP TYPE  IF EXISTS vendor_status;