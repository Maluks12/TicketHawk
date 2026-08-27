-- ── Event information: ID required becomes a yes/no, contact is retired ─────
--
-- Three changes, all to the catalogue. No table or column changes, and no Go
-- changes: enum validation, rendering and length limits are all already driven
-- by catalogue rows, so this migration is the whole feature.
--
-- 1. id_required stops being prose and becomes a Yes/No enum, so the app can
--    show it as a summary tile beside age and doors. Tile eligibility on the
--    client is "value_type is enum or time", which this makes true for it.
--
-- 2. id_accepted is a new prose field for WHICH documents are accepted. Split
--    into its own key rather than packing two answers into one value, because
--    the yes/no has to be readable on its own to render as a tile, and a
--    parsed-out prefix is a value nobody can validate.
--
-- 3. organiser_contact is deactivated rather than deleted. event_info_items
--    references the catalogue with ON DELETE RESTRICT precisely so retiring a
--    field cannot take live event data with it, and is_active is the supported
--    way to hide one from the picker.

-- ── 1. id_required → enum ────────────────────────────────────────────────────
-- value_type and enum_options must move together: chk_info_enum_options
-- requires a non-empty enum_options for an enum row, so setting either alone
-- would fail. One UPDATE satisfies it because the constraint is evaluated at
-- statement end.
UPDATE event_info_catalogue
SET    value_type   = 'enum',
       enum_options = ARRAY['Yes', 'No'],
       hint         = 'Will attendees need to show ID at the door?'
WHERE  key = 'id_required';

-- Any stored value predates the enum and is prose, so it is not one of Yes/No
-- and would render as a tile containing a sentence. There are no events yet,
-- which makes this a no-op today; it is here so the migration is still correct
-- if it is ever run against a database that does have rows.
DELETE FROM event_info_items
WHERE  info_key = 'id_required'
AND    value NOT IN ('Yes', 'No');

-- ── 2. id_accepted ───────────────────────────────────────────────────────────
-- display_order 35 sits it directly after id_required (30) and before
-- dress_code (40), so the two ID questions read as one pair in the form.
INSERT INTO event_info_catalogue
    (key, label, hint, value_type, enum_options, display_order, review_required_when_published)
VALUES
    ('id_accepted', 'Accepted ID',
     'Which documents you will accept — SA ID, driver''s licence, passport. Only needed if ID is required.',
     'text', NULL, 35, FALSE)
ON CONFLICT (key) DO NOTHING;

-- ── 3. Retire organiser_contact ──────────────────────────────────────────────
UPDATE event_info_catalogue
SET    is_active = FALSE
WHERE  key = 'organiser_contact';