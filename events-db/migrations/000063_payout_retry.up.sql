-- ============================================================================
-- Bounded retry for failed payout transfers.
--
-- Before this migration 'failed' was a terminal state. FetchPayableRows only
-- selected 'pending' and 'processing', so a transfer that failed for ANY
-- reason — a momentarily empty balance, a network blip, a Paystack 5xx — left
-- the organiser's money stranded permanently and silently. The only recovery
-- was someone noticing and running UPDATE by hand.
--
-- That is a bad failure mode for money the platform is holding on behalf of
-- someone else: it is invisible, it is indefinite, and the person harmed by it
-- is the one least able to detect it.
--
-- The fix is a retry schedule with a cap. Transient failures heal themselves;
-- a genuinely broken recipient stops retrying after MaxPayoutAttempts and
-- stays 'failed' with next_attempt_at NULL, which is the signal for a human.
-- ============================================================================

ALTER TABLE organizer_payouts
    ADD COLUMN IF NOT EXISTS attempts        INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN organizer_payouts.attempts IS
    'Transfer attempts made so far. Incremented by MarkPayoutFailed.';

COMMENT ON COLUMN organizer_payouts.next_attempt_at IS
    'When the worker may retry a failed row. NULL on a failed row means the '
    'retry budget is exhausted and the row needs manual attention.';

-- The worker polls this every 10 minutes; keep it cheap. Partial index, so it
-- only covers rows actually awaiting a retry rather than the whole table.
CREATE INDEX IF NOT EXISTS idx_payouts_retry_due
    ON organizer_payouts (next_attempt_at)
    WHERE status = 'failed' AND next_attempt_at IS NOT NULL;

-- Existing failures predate the retry machinery and would otherwise stay
-- stranded forever. Give them one immediate attempt; from there the normal
-- backoff applies. refund_clawback rows are debt owed TO the platform and are
-- never transferred, so they are excluded — FetchPayableRows filters them too,
-- but setting the column would be misleading in the table.
UPDATE organizer_payouts
SET next_attempt_at = CURRENT_TIMESTAMP
WHERE status = 'failed'
  AND kind <> 'refund_clawback';