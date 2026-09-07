-- Reverse 000087.
--
-- Less destructive than 000086's down-migration: no ledger rows are dropped, so
-- the record of money taken and spent survives. What IS lost is the offline
-- provenance on existing transactions — device signatures, device sequence
-- numbers, and the accumulator readings — which is the evidence you would need
-- to investigate a disputed offline charge later.
--
-- So: safe on a scratch database and after a clean event, worth a dump first if
-- any offline transactions have actually settled.
--
-- Dropping wallet_shortfalls discards in-flight recovery state. Any wallet
-- still negative stays negative; re-running the up-migration does not
-- re-detect them, because detection happens at reconciliation. Resolve open
-- shortfalls before reversing this.

DROP TABLE IF EXISTS offline_spend_refusals;

DROP TABLE IF EXISTS wallet_shortfalls;
DROP TYPE  IF EXISTS shortfall_state;

DROP INDEX IF EXISTS idx_cashless_offline_unsigned;
DROP INDEX IF EXISTS uq_cashless_device_seq;

ALTER TABLE cashless_transactions
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS accum_after_cents,
    DROP COLUMN IF EXISTS accum_before_cents,
    DROP COLUMN IF EXISTS device_seq,
    DROP COLUMN IF EXISTS device_signature;

ALTER TABLE vendor_pos_devices
    DROP CONSTRAINT IF EXISTS chk_pos_offline_requires_attestation;

ALTER TABLE vendor_pos_devices
    DROP COLUMN IF EXISTS signing_pubkey,
    DROP COLUMN IF EXISTS attestation_fail_count,
    DROP COLUMN IF EXISTS attested_at,
    DROP COLUMN IF EXISTS offline_enabled;

DROP TABLE IF EXISTS event_cashless_config;
DROP TYPE  IF EXISTS offline_spend_policy;