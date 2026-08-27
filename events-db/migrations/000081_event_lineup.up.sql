-- ============================================================================
-- 000081: Event line-up — performers and teams, with images.
--
-- WHY A TABLE
--
-- events.performers and events.teams are text[]. A name is all they can hold,
-- and the requirement is a name PLUS an image, a stage, a set time and a link.
-- Adding three more parallel arrays would make position the only thing tying
-- them together, so deleting one performer would mean re-indexing four arrays
-- in lockstep and any mismatch would silently pair the wrong image with the
-- wrong name.
--
-- WHY ONE TABLE WITH A `kind` DISCRIMINATOR AND NOT TWO
--
-- Performers and teams are the same shape — a name, a picture, an optional
-- link — and differ only in how the client lays them out: the app already
-- branches on `isASport` to render teams as a fixture and performers as a
-- list. That is a presentation decision, and it does not need two tables,
-- two repositories and two endpoints behind it.
--
-- One table also means an event can legitimately have both, which the current
-- schema allows and the app quietly ignores: a charity match with a DJ after
-- the final whistle has teams AND performers, and today the app shows only the
-- teams because isASport is a category check rather than a data check.
--
-- WHY THE OLD COLUMNS SURVIVE
--
-- events.performers and events.teams are NOT dropped, and are still written on
-- every line-up save (see ReplaceEventLineup's dual write). Shipped mobile
-- builds read those arrays directly — EventsApp/app/event/[id].tsx lines
-- 401-402 — and a user on an old build does not update because we deployed.
-- Dropping the columns would empty the line-up section on every phone that has
-- not been updated.
--
-- They become read-only-for-legacy-clients after phase 5 switches the payload
-- over, and can be dropped a release after the app's minimum supported version
-- moves past that point. Not before.
-- ============================================================================

CREATE TABLE IF NOT EXISTS event_lineup (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id   UUID        NOT NULL REFERENCES events(id) ON DELETE CASCADE,

    -- 'performer' — artist, speaker, act
    -- 'team'      — the two sides of a fixture
    kind       VARCHAR(12) NOT NULL DEFAULT 'performer',

    name       VARCHAR(120) NOT NULL,
    image_url  TEXT,

    -- Free line under the name. "Main stage", "Home", "Headliner", "Opening
    -- act". Deliberately not an enum: the vocabulary differs per event type and
    -- constraining it would mean a migration every time somebody runs a format
    -- we did not anticipate.
    subtitle   VARCHAR(80),

    stage      VARCHAR(80),

    -- Full timestamp, not a time-of-day. Most festivals cross midnight, so a
    -- bare 01:30 is ambiguous about which day it belongs to — and getting that
    -- wrong puts the headliner 24 hours out on the schedule.
    set_time   TIMESTAMPTZ,

    -- The act's own page: Spotify, Instagram, their site. Validated in Go
    -- against the same host rules as organiser links. NOT a link to a profile
    -- on this platform — performers are not accounts here, and pretending
    -- otherwise would mean an entity model nobody has asked for.
    external_url TEXT,

    sort_order SMALLINT    NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_lineup_kind CHECK (kind IN ('performer', 'team')),
    CONSTRAINT chk_lineup_name_len CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    -- Backstop only; the Go validator is the real control and gives a usable
    -- error message.
    CONSTRAINT chk_lineup_external_url CHECK (
        external_url IS NULL OR external_url ~* '^https://'
    )
);

CREATE INDEX IF NOT EXISTS idx_event_lineup_event
    ON event_lineup(event_id, kind, sort_order);

DROP TRIGGER IF EXISTS update_event_lineup_updated_at ON event_lineup;
CREATE TRIGGER update_event_lineup_updated_at
    BEFORE UPDATE ON event_lineup
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ── Backfill from the existing arrays ───────────────────────────────────────
--
-- WITH ORDINALITY preserves array position as sort_order, so the line-up comes
-- out in the order the organiser typed it rather than whatever order the unnest
-- happens to produce.
--
-- Names are trimmed and blanks skipped: the arrays have no CHECK constraint, so
-- a trailing comma in the organiser form has been producing empty entries.
--
-- Guarded by NOT EXISTS so re-running is safe and so it never overwrites a
-- line-up someone has already built through the new endpoint.

INSERT INTO event_lineup (event_id, kind, name, sort_order)
SELECT e.id, 'performer', btrim(p.name), (p.ord - 1)::smallint
FROM   events e
CROSS  JOIN LATERAL unnest(e.performers) WITH ORDINALITY AS p(name, ord)
WHERE  e.performers IS NOT NULL
  AND  btrim(p.name) <> ''
  AND  NOT EXISTS (SELECT 1 FROM event_lineup l WHERE l.event_id = e.id);

INSERT INTO event_lineup (event_id, kind, name, sort_order)
SELECT e.id, 'team', btrim(t.name), (t.ord - 1)::smallint
FROM   events e
CROSS  JOIN LATERAL unnest(e.teams) WITH ORDINALITY AS t(name, ord)
WHERE  e.teams IS NOT NULL
  AND  btrim(t.name) <> ''
  AND  NOT EXISTS (
         SELECT 1 FROM event_lineup l
         WHERE l.event_id = e.id AND l.kind = 'team'
       );