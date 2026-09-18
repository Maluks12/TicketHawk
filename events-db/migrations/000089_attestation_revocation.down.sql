-- Reverse 000089.
--
-- Safe. This is a cache of a public list — dropping it loses nothing that a
-- single refresh cannot rebuild.
--
-- One operational note: while these tables are absent, the revocation check has
-- no data to work against. Whether that means "allow" or "refuse" is decided in
-- the service, not here, and it refuses once the cached copy is stale. So
-- reversing this migration on a running system stops enrolment rather than
-- silently weakening it — which is the correct direction, but do it knowing
-- that tills cannot enrol until it is restored.

DROP TABLE IF EXISTS attestation_revocation_meta;
DROP TABLE IF EXISTS attestation_revocations;