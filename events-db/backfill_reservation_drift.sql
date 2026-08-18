-- backfill_reservation_drift.sql
--
-- Rebuilds reserved_count on events and ticket_types from the live
-- ticket_reservations rows, clearing accumulated phantom capacity.
--
-- ORDER OF OPERATIONS MATTERS:
--   1. Run diagnose_reservation_drift.sql and keep the output.
--   2. Deploy the fixed reservation_repository.go FIRST. Backfilling before
--      the code fix just gives the leak a clean slate to refill.
--   3. Run this during a quiet window. It takes row locks on events and
--      ticket_types, which checkout also locks FOR UPDATE.
--   4. Re-run the diagnose script to confirm drift is zero.
--
--   docker run --rm -i --env-file ./.env.production postgres:16-alpine \
--     sh -c 'psql "$DATABASE_URL" -f -' < backfill_reservation_drift.sql

BEGIN;

-- Clear expired reservation rows first so they cannot inflate the rebuild.
DELETE FROM ticket_reservations WHERE expires_at <= NOW();

-- Rebuild ticket_types.reserved_count from surviving rows.
UPDATE ticket_types tt
SET reserved_count = COALESCE(agg.qty, 0),
    updated_at     = NOW()
FROM (
    SELECT tt2.id AS ticket_type_id,
           COALESCE(SUM(tr.quantity), 0)::int AS qty
    FROM ticket_types tt2
    LEFT JOIN ticket_reservations tr
           ON tr.ticket_type_id = tt2.id
          AND tr.expires_at > NOW()
    GROUP BY tt2.id
) agg
WHERE tt.id = agg.ticket_type_id
  AND tt.reserved_count IS DISTINCT FROM agg.qty;

-- Rebuild events.reserved_count from surviving rows.
UPDATE events e
SET reserved_count = COALESCE(agg.qty, 0),
    updated_at     = NOW()
FROM (
    SELECT e2.id AS event_id,
           COALESCE(SUM(tr.quantity), 0)::int AS qty
    FROM events e2
    LEFT JOIN ticket_reservations tr
           ON tr.event_id = e2.id
          AND tr.expires_at > NOW()
    GROUP BY e2.id
) agg
WHERE e.id = agg.event_id
  AND e.reserved_count IS DISTINCT FROM agg.qty;

-- Sanity gate: nothing should be negative or exceed capacity after the rebuild.
DO $$
DECLARE bad int;
BEGIN
    SELECT COUNT(*) INTO bad FROM events WHERE reserved_count < 0;
    IF bad > 0 THEN RAISE EXCEPTION 'events.reserved_count negative on % rows', bad; END IF;

    SELECT COUNT(*) INTO bad FROM ticket_types WHERE reserved_count < 0;
    IF bad > 0 THEN RAISE EXCEPTION 'ticket_types.reserved_count negative on % rows', bad; END IF;
END $$;

-- Inspect these before committing.
SELECT 'events with reservations'      AS scope, COUNT(*) FROM events        WHERE reserved_count > 0
UNION ALL
SELECT 'ticket_types with reservations', COUNT(*) FROM ticket_types  WHERE reserved_count > 0
UNION ALL
SELECT 'live reservation rows',          COUNT(*) FROM ticket_reservations WHERE expires_at > NOW();

COMMIT;