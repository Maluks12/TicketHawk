-- ============================================================================
-- 000093: Per-organiser fee arrangements.
--
-- WHAT THIS ADDS
--
-- Two optional OVERRIDES on organizers, set by a master admin when a specific
-- commercial arrangement has been agreed with an organiser:
--
--   custom_commission_pct   Organiser-paid commission, as a PERCENT of the
--                           ticket subtotal (4.50 = 4.5%). Deducted from the
--                           organiser's payout. Lands in bookings.organizer_fee.
--   custom_service_fee      Buyer-paid service fee in ZAR PER TICKET, before the
--                           20%-of-face-value cap. Lands in bookings.platform_fee.
--
-- NULL means "no arrangement — use the platform default" (5% and R10 as of this
-- migration, defined in repository/booking_repository.go). That is the
-- load-bearing choice: an organiser without a deal follows any future change to
-- the default automatically, instead of being frozen at whatever was copied in
-- on the day this ran.
--
-- WHY NOT THE EXISTING organizers.commission_rate COLUMN
--
-- It dates from 000003, defaults to 10.00 and has never been read by the fee
-- code. Every existing row holds a value nobody chose. Reusing it would have
-- meant first rewriting every row, and a missed row would silently charge 10%.
-- It is left alone.
--
-- LIMITS
--
-- Commission: 4.00% to 10.00%. The floor is there because the platform bears
-- Paystack's processing fee (bearer = "account" in the split), so a commission
-- below the processing cost loses money on every sale. The ceiling stops a typo
-- (45 instead of 4.5) from reaching a live event.
--
-- Service fee: R0 to R10 per ticket. It can be lowered for an organiser but
-- never raised above the R10 default.
--
-- The same limits are enforced in Go (repository.ValidateFeeOverride). The
-- constraints here are the backstop, not the primary check.
--
-- WHAT A CHANGE AFFECTS
--
-- Only bookings created AFTER the change. Each booking stores its own
-- platform_fee and organizer_fee at creation; payouts, splits, refunds and
-- analytics all read those stored amounts, never the rate.
--
-- AUDIT
--
-- Every change is written to staff_audit_log (action 'organizer.fees_update')
-- with the old and new values, inside the same transaction as the update.
-- ============================================================================

ALTER TABLE organizers
    ADD COLUMN IF NOT EXISTS custom_commission_pct  NUMERIC(5,2),
    ADD COLUMN IF NOT EXISTS custom_service_fee     NUMERIC(6,2),
    ADD COLUMN IF NOT EXISTS custom_fees_note       TEXT,
    ADD COLUMN IF NOT EXISTS custom_fees_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS custom_fees_updated_by UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE organizers DROP CONSTRAINT IF EXISTS chk_custom_commission_pct;
ALTER TABLE organizers ADD CONSTRAINT chk_custom_commission_pct
    CHECK (custom_commission_pct IS NULL
           OR (custom_commission_pct >= 4.00 AND custom_commission_pct <= 10.00));

ALTER TABLE organizers DROP CONSTRAINT IF EXISTS chk_custom_service_fee;
ALTER TABLE organizers ADD CONSTRAINT chk_custom_service_fee
    CHECK (custom_service_fee IS NULL
           OR (custom_service_fee >= 0 AND custom_service_fee <= 10.00));

COMMENT ON COLUMN organizers.custom_commission_pct IS
    'Negotiated organiser commission in percent (4.50 = 4.5%). NULL = platform default. '
    'Range 4.00-10.00. Master admin only. Affects bookings created after the change.';
COMMENT ON COLUMN organizers.custom_service_fee IS
    'Negotiated buyer service fee in ZAR per ticket, before the 20% face-value cap. '
    'NULL = platform default. Range 0-10.';
COMMENT ON COLUMN organizers.custom_fees_note IS
    'Why the arrangement exists, as entered by the master admin who set it.';
COMMENT ON COLUMN organizers.commission_rate IS
    'LEGACY, UNUSED. Never read by the fee code. See custom_commission_pct (000093).';