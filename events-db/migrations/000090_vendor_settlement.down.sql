-- Reverse 000090.
--
-- Safe in the sense that no ledger data is lost — every figure in these tables
-- is DERIVED from cashless_transactions and can be recalculated.
--
-- But not consequence-free. vendor_settlements records what a vendor was told
-- they were owed and what was actually paid, including the commission rate
-- APPLIED AT THE TIME. Recalculating after a rate change reproduces different
-- numbers, so dropping this loses the only record of the figures both sides
-- agreed to. If any settlement has reached 'paid', take a dump first.

DROP TABLE IF EXISTS event_float_reconciliations;

DROP TABLE IF EXISTS vendor_settlements;
DROP TYPE  IF EXISTS settlement_status;