-- ============================================================================
-- 000050: Event staffing — scanner assignment & join codes.
--
-- Replaces the single hard-coded "scanner must equal events.organizer_id"
-- check with a membership model:
--   * event_staff        — who may scan (scanner) or also manage staff (manager)
--                          for a given event. The event OWNER (events.organizer_id)
--                          is implicit and never inserted here.
--   * event_staff_codes  — short-lived, use-capped join codes so door staff can
--                          be onboarded at the venue in seconds without any
--                          credential sharing.
--   * ticket_assignments.scanned_by_user_id — honest audit of WHO scanned;
--                          the old scanned_by_organizer_id stays populated when
--                          the scanner happens to have an organizer profile.
-- Additive only: no backfill, no behavioural change for existing events.
-- ============================================================================

CREATE TYPE staff_role   AS ENUM ('scanner', 'manager');
CREATE TYPE staff_status AS ENUM ('active', 'revoked');

-- Codes first: event_staff.joined_via references this table.
CREATE TABLE event_staff_codes (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id     UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    code         VARCHAR(12) NOT NULL UNIQUE,        -- 8-char Crockford base32, stored without dashes
    grants_role  staff_role NOT NULL DEFAULT 'scanner',
    max_uses     INT  NOT NULL DEFAULT 10 CHECK (max_uses > 0),
    use_count    INT  NOT NULL DEFAULT 0  CHECK (use_count >= 0),
    expires_at   TIMESTAMPTZ NOT NULL,
    created_by   UUID NOT NULL REFERENCES users(id),
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_staff_codes_event ON event_staff_codes (event_id);

CREATE TABLE event_staff (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    role        staff_role   NOT NULL DEFAULT 'scanner',
    status      staff_status NOT NULL DEFAULT 'active',
    added_by    UUID NOT NULL REFERENCES users(id),
    joined_via  UUID REFERENCES event_staff_codes(id), -- NULL when added directly by email
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ,
    revoked_by  UUID REFERENCES users(id),
    UNIQUE (event_id, user_id)
);

-- Hot path: the per-scan authorization lookup.
CREATE INDEX idx_event_staff_lookup
    ON event_staff (event_id, user_id) WHERE status = 'active';

-- "Which events can I scan?" (drives the staffer's event list).
CREATE INDEX idx_event_staff_user
    ON event_staff (user_id) WHERE status = 'active';

-- Honest scan audit: the actual human who performed the scan, regardless of
-- whether they have an organizer business profile.
ALTER TABLE ticket_assignments
    ADD COLUMN IF NOT EXISTS scanned_by_user_id UUID REFERENCES users(id);