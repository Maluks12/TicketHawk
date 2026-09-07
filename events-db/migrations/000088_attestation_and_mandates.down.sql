-- Reverse 000088.
--
-- One thing here does NOT reverse: ALTER TYPE ... ADD VALUE cannot be undone in
-- Postgres. 'awaiting_settlement' stays in the shortfall_state enum after this
-- runs. Harmless — nothing references it once wallet_shortfalls loses its
-- columns — but worth knowing before you assume this is a clean round trip.
-- Removing it properly means recreating the type and every column using it,
-- which is not worth doing for a spare enum label.
--
-- Dropping payment_mandates DESTROYS CONSENT EVIDENCE. Those rows are what
-- answers an attendee disputing a recovery charge. Take a dump first if any
-- mandate has ever been used.

DROP INDEX IF EXISTS uq_shortfall_settle_token;

ALTER TABLE wallet_shortfalls
    DROP COLUMN IF EXISTS settle_link_token,
    DROP COLUMN IF EXISTS settle_link_sent_at,
    DROP COLUMN IF EXISTS mandate_id;

DROP TABLE IF EXISTS payment_mandates;
DROP TYPE  IF EXISTS mandate_status;

DROP TABLE IF EXISTS pos_key_grants;
DROP TABLE IF EXISTS pos_attestation_nonces;
DROP TABLE IF EXISTS pos_device_attestations;
DROP TYPE  IF EXISTS attestation_verdict;

-- NOTE: shortfall_state keeps 'awaiting_settlement'. See the header.