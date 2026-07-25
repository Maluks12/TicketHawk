-- Google Places `place_id` for events.
--
-- Why: the clients built map links as a fuzzy TEXT search
-- (`maps/search/?api=1&query=<venue>, <city>`), which sends buyers to whatever
-- Google guesses — often a completely different venue, especially for generic
-- names or a venue/city mismatch like "FNB Stadium, Sandton".
--
-- Storing the place_id chosen in the organiser's autocomplete lets every client
-- link with `&query_place_id=<id>`, which pins the EXACT place Google itself
-- resolved. latitude/longitude already exist (migration 000004) and are now
-- actually populated by the same autocomplete selection.
ALTER TABLE events
    ADD COLUMN IF NOT EXISTS place_id TEXT;

COMMENT ON COLUMN events.place_id IS
    'Google Places place_id captured at event creation; used for exact map links.';