-- migrations/000060_refund_clawback.down.sql
--
-- Rolling back drops the record of outstanding organiser debt. Any clawback
-- rows already in organizer_payouts would violate the restored kind CHECK, so
-- they are removed first — which means that debt is forgiven, not deferred.
-- Check for outstanding balances before running this:
--
--   SELECT business_name, payout_debt FROM organizers WHERE payout_debt > 0;

DELETE FROM organizer_payouts WHERE kind = 'refund_clawback';

ALTER TABLE organizer_payouts
    DROP CONSTRAINT IF EXISTS organizer_payouts_kind_check;
ALTER TABLE organizer_payouts
    ADD CONSTRAINT organizer_payouts_kind_check
    CHECK (kind IN ('event_settlement', 'reserve_release'));

DROP INDEX IF EXISTS idx_organizers_subaccount_invalid;

ALTER TABLE organizers
    DROP CONSTRAINT IF EXISTS chk_payout_debt_non_negative;
ALTER TABLE organizers
    DROP COLUMN IF EXISTS payout_debt,
    DROP COLUMN IF EXISTS subaccount_invalid_at;

DROP TABLE IF EXISTS payment_disputes;