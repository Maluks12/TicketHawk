-- ============================================================================
-- 000080: Event information — a catalogue the organiser picks from.
--
-- WHY A TABLE AND NOT NULLABLE COLUMNS ON events
--
-- "The organiser chooses which fields to include" is the requirement, and
-- columns cannot express it. With columns, every field exists on every event
-- and the only signal that one was not chosen is NULL — which is
-- indistinguishable from chosen-but-left-blank. That is exactly the bug
-- visible in the Howler screenshots: "PARKING" and "PROHIBITED ITEMS" render
-- as headings with nothing under them, because the page has no way to know the
-- organiser never opted in.
--
-- With a row per chosen field, an unchosen field has no row and literally
-- cannot render. The organiser also gets ordering for free, and adding a new
-- field type later is a seed row rather than a migration plus a deploy of four
-- frontends.
--
-- The cost is a join and losing the ability to filter events on, say, age
-- restriction in SQL. That is why age and doors are TYPED (see value_type) —
-- if a browse-by-age-restriction surface is ever built, those two are the only
-- ones that would need it, and their values are constrained rather than prose.
--
-- WHY refund_policy AND terms_and_conditions STAY AS COLUMNS
--
-- They already exist, they are legally significant, and they are referenced by
-- the booking and refund flows. Moving them into a generic key/value table
-- would make them findable only by string key, which is the wrong property for
-- the two fields most likely to be produced in a dispute. They render in the
-- same Information block on the client; that is a presentation decision, not a
-- storage one.
-- ============================================================================

-- ── Catalogue ───────────────────────────────────────────────────────────────
--
-- Server-owned. The organiser picks from this list; they do not invent keys.
-- One free-form "additional_info" entry covers the genuine long tail without
-- the label chaos of open custom fields, where the same concept arrives as
-- "Info", "info" and "IMPORTANT!!" on three different events.

CREATE TABLE IF NOT EXISTS event_info_catalogue (
    key           VARCHAR(48) PRIMARY KEY,

    -- Default label shown to organiser and visitor. Clients may localise;
    -- this is the fallback and the admin-facing name.
    label         VARCHAR(80)  NOT NULL,

    -- Shown under the field in the organiser form. Nudges toward a useful
    -- answer — the difference between "Parking" and "Parking: where should
    -- people park, and does it cost anything?" is most of the quality.
    hint          VARCHAR(240),

    -- 'text'  — free prose, rendered as-is
    -- 'enum'  — one of enum_options below
    -- 'time'  — HH:MM, rendered in event-local time by the client
    value_type    VARCHAR(12)  NOT NULL DEFAULT 'text',

    -- Allowed values when value_type = 'enum'. NULL otherwise.
    enum_options  TEXT[],

    -- Default position in the Information block. The organiser may override
    -- per event via event_info_items.sort_order.
    display_order SMALLINT     NOT NULL DEFAULT 100,

    -- ⚠️  THE FIELD THAT DECIDES WHETHER A LIVE EDIT NEEDS ADMIN APPROVAL.
    --
    -- TRUE means: once the event is published, this value can only be changed
    -- through the event_edit request flow, not by the organiser directly.
    --
    -- Only age_requirement carries it. Someone who bought a ticket to an
    -- "all ages" event and arrives with a 15-year-old to an 18+ door has a
    -- refund dispute, and the organiser should not be able to create that
    -- silently after the sale.
    --
    -- Everything else is free to change live, and deliberately so: parking
    -- arrangements, prohibited items and transport notes CHANGE in the run-up
    -- to an event, and routing a "the north gate is now closed" update through
    -- an admin queue would mean the accurate version arrives after the event.
    review_required_when_published BOOLEAN NOT NULL DEFAULT FALSE,

    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_info_value_type CHECK (value_type IN ('text', 'enum', 'time')),
    -- An enum field with no options is unfillable; a text field with options
    -- is a sign someone meant to set value_type and forgot.
    CONSTRAINT chk_info_enum_options CHECK (
        (value_type = 'enum' AND enum_options IS NOT NULL AND array_length(enum_options, 1) > 0)
        OR (value_type <> 'enum' AND enum_options IS NULL)
    )
);

-- ── Per-event chosen values ─────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS event_info_items (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id   UUID        NOT NULL REFERENCES events(id) ON DELETE CASCADE,

    -- RESTRICT, not CASCADE: retiring a catalogue entry must not silently
    -- delete information from live events. Deactivate it with is_active
    -- instead, which hides it from the picker while leaving existing events
    -- intact.
    info_key   VARCHAR(48) NOT NULL REFERENCES event_info_catalogue(key) ON DELETE RESTRICT,

    value      TEXT        NOT NULL,
    sort_order SMALLINT    NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_event_info_item UNIQUE (event_id, info_key),

    -- An empty value is not a chosen field, it is a field that should have no
    -- row. Enforced here so the "unchosen fields cannot render" property holds
    -- no matter what writes the row.
    CONSTRAINT chk_event_info_value_len CHECK (char_length(btrim(value)) BETWEEN 1 AND 600)
);

CREATE INDEX IF NOT EXISTS idx_event_info_items_event
    ON event_info_items(event_id, sort_order);

DROP TRIGGER IF EXISTS update_event_info_items_updated_at ON event_info_items;
CREATE TRIGGER update_event_info_items_updated_at
    BEFORE UPDATE ON event_info_items
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ── Seed ────────────────────────────────────────────────────────────────────
--
-- Ordered to match how a visitor reads the block: the hard constraints first
-- (can I get in, when, with what), then logistics, then the long tail.
--
-- ON CONFLICT DO NOTHING so re-running is safe and so an admin who has edited
-- a label in production does not have it reverted by a redeploy.

INSERT INTO event_info_catalogue
    (key, label, hint, value_type, enum_options, display_order, review_required_when_published)
VALUES
    ('age_requirement', 'Age requirement',
     'Who can attend? This cannot be changed without review once tickets are on sale.',
     'enum', ARRAY['All ages', 'Under 18s with an adult', '13+', '16+', '18+', '21+'], 10, TRUE),

    ('doors_open', 'Doors open',
     'What time can people start arriving? Leave blank if it is the same as the start time.',
     'time', NULL, 20, FALSE),

    ('id_required', 'ID required',
     'Will attendees need to show identification at the door?',
     'text', NULL, 30, FALSE),

    ('dress_code', 'Dress code',
     'Anything attendees need to wear — or must not wear.',
     'text', NULL, 40, FALSE),

    ('parking', 'Parking',
     'Where should people park, is it free, and which entrance should they use?',
     'text', NULL, 50, FALSE),

    ('transport', 'Getting there',
     'Nearest taxi rank, Gautrain station, shuttle arrangements or ride-hailing pickup point.',
     'text', NULL, 60, FALSE),

    ('accessibility', 'Accessibility',
     'Wheelchair access, accessible bathrooms, seating for people who cannot stand for long.',
     'text', NULL, 70, FALSE),

    ('prohibited_items', 'Prohibited items',
     'What may not be brought in — glass, outside food, professional cameras, and so on.',
     'text', NULL, 80, FALSE),

    ('bag_policy', 'Bag policy',
     'Bag size limits, whether bags are searched, whether there is a cloakroom.',
     'text', NULL, 90, FALSE),

    ('food_and_drink', 'Food and drink',
     'What is available on site, and whether attendees may bring their own.',
     'text', NULL, 100, FALSE),

    ('cashless', 'Payment on site',
     'Card only, cash accepted, or a wristband top-up system.',
     'text', NULL, 110, FALSE),

    ('re_entry', 'Re-entry',
     'Can attendees leave and come back? Say so either way — people ask.',
     'text', NULL, 120, FALSE),

    ('seating', 'Seating',
     'Seated, standing, general admission, or bring your own chair.',
     'text', NULL, 130, FALSE),

    ('child_policy', 'Children',
     'Whether children need their own ticket, and any age below which they go free.',
     'text', NULL, 140, FALSE),

    ('weather_policy', 'Weather',
     'What happens if it rains — does the event go ahead, move, or get postponed?',
     'text', NULL, 150, FALSE),

    ('load_shedding', 'Load shedding',
     'Backup power arrangements, or what happens if the power goes out mid-event.',
     'text', NULL, 160, FALSE),

    ('what_to_bring', 'What to bring',
     'Anything attendees should bring with them — a mat, a blanket, sunscreen, a jacket.',
     'text', NULL, 170, FALSE),

    ('camping', 'Camping',
     'Whether camping is available, what it costs, and what is provided.',
     'text', NULL, 180, FALSE),

    ('lockers', 'Lockers and storage',
     'Whether lockers are available and what they cost.',
     'text', NULL, 190, FALSE),

    ('lost_property', 'Lost property',
     'Where to go on the day, and how to follow up afterwards.',
     'text', NULL, 200, FALSE),

    ('organiser_contact', 'Contact on the day',
     'A number or email attendees can use if something goes wrong at the event.',
     'text', NULL, 210, FALSE),

    ('additional_info', 'Additional information',
     'Anything else attendees should know that does not fit the fields above.',
     'text', NULL, 900, FALSE)
ON CONFLICT (key) DO NOTHING;