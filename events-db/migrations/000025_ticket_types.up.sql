-- migrations/000025_ticket_types.up.sql
--
-- Ticket Types: named pricing tiers per event (GA, VIP, Early Bird, etc.)
-- events.price is kept in sync by a trigger to always reflect the cheapest
-- visible type. When an event has no types, price stays as the organiser set it.

CREATE TABLE ticket_types (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id       UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,

    -- Display
    name           VARCHAR(100) NOT NULL,
    description    TEXT,

    -- Pricing & availability
    price          DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    capacity       INTEGER,                      -- NULL = unlimited for this tier
    tickets_sold   INTEGER NOT NULL DEFAULT 0,
    reserved_count INTEGER NOT NULL DEFAULT 0,

    -- Ordering & visibility
    sort_order     INTEGER NOT NULL DEFAULT 0,
    is_visible     BOOLEAN NOT NULL DEFAULT TRUE,

    -- Optional sale window (NULL = always on sale while visible)
    sale_starts_at TIMESTAMPTZ,
    sale_ends_at   TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_tt_price_positive    CHECK (price >= 0),
    CONSTRAINT chk_tt_capacity_positive CHECK (capacity IS NULL OR capacity > 0),
    CONSTRAINT chk_tt_sold_valid        CHECK (tickets_sold >= 0),
    CONSTRAINT chk_tt_reserved_valid    CHECK (reserved_count >= 0),
    CONSTRAINT chk_tt_not_oversold      CHECK (
        capacity IS NULL OR tickets_sold + reserved_count <= capacity
    )
);

CREATE INDEX idx_ticket_types_event_id ON ticket_types(event_id);
CREATE INDEX idx_ticket_types_event_visible
    ON ticket_types(event_id, sort_order)
    WHERE is_visible = TRUE;

CREATE TRIGGER update_ticket_types_updated_at
    BEFORE UPDATE ON ticket_types
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ---------------------------------------------------------------------------
-- Trigger: keep events.price = MIN(visible ticket type prices) automatically.
-- When the last type is deleted events.price falls back to 0; the organiser
-- should update it manually if they later want a flat price shown.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION sync_event_price_from_ticket_types()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    v_event_id UUID;
BEGIN
    -- For DELETE, NEW is NULL — use OLD
    v_event_id := COALESCE(NEW.event_id, OLD.event_id);

    UPDATE events
    SET price = COALESCE(
        (SELECT MIN(price)
         FROM ticket_types
         WHERE event_id = v_event_id
           AND is_visible = TRUE),
        0
    ),
    updated_at = NOW()
    WHERE id = v_event_id;

    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_sync_event_price
    AFTER INSERT OR UPDATE OR DELETE ON ticket_types
    FOR EACH ROW
    EXECUTE FUNCTION sync_event_price_from_ticket_types();