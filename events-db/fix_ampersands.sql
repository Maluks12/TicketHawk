-- ============================================================================
-- Repair seeded titles and descriptions that contain a literal "&amp;".
--
-- My mistake in the seed script: I wrote ampersands as the HTML entity
-- "&amp;" because I had been writing HTML earlier in the same session. SQL has
-- no such escaping — an ampersand is just a character — so the entity was
-- stored verbatim and the app renders "Jazz &amp; Poetry Evening".
--
-- Run this instead of re-seeding if you already have bookings, follows or
-- notifications pointing at these events. It touches text only; no ids move.
-- ============================================================================

BEGIN;

UPDATE events
SET    title                = replace(title, '&amp;', '&'),
       description          = replace(description, '&amp;', '&'),
       venue_name           = replace(venue_name, '&amp;', '&'),
       refund_policy        = replace(refund_policy, '&amp;', '&'),
       terms_and_conditions = replace(terms_and_conditions, '&amp;', '&')
WHERE  title       LIKE '%&amp;%'
   OR  description LIKE '%&amp;%'
   OR  venue_name  LIKE '%&amp;%';

UPDATE event_info_items
SET    value = replace(value, '&amp;', '&')
WHERE  value LIKE '%&amp;%';

UPDATE event_lineup
SET    name = replace(name, '&amp;', '&')
WHERE  name LIKE '%&amp;%';

UPDATE organizers
SET    business_name = replace(business_name, '&amp;', '&'),
       description   = replace(description, '&amp;', '&')
WHERE  business_name LIKE '%&amp;%'
   OR  description   LIKE '%&amp;%';

-- ── Slugs ───────────────────────────────────────────────────────────────────
--
-- seed_slug() strips non-alphanumerics, so "&amp;" collapsed to "-amp-" and
-- produced URLs like /events/breathwork-amp-cold-immersion-e5000000. Those
-- resolve fine, so this is cosmetic — but the slug is a PUBLIC url that ends up
-- in shared links and search results, and "amp" in the middle of one looks like
-- corruption.
--
-- Scoped to seeded rows only (id prefix e5000000). Outside that set a "-amp-"
-- could be a legitimate word — "amp-hour", a band called AMP — and this would
-- silently rewrite it.
UPDATE events
SET    slug = replace(slug, '-amp-', '-')
WHERE  id::text LIKE 'e5000000-%'
  AND  slug LIKE '%-amp-%';

COMMIT;

-- Should return no rows.
SELECT 'events' AS tbl, id::text, title FROM events WHERE title LIKE '%&amp;%'
UNION ALL
SELECT 'info', event_id::text, value FROM event_info_items WHERE value LIKE '%&amp;%'
UNION ALL
SELECT 'lineup', event_id::text, name FROM event_lineup WHERE name LIKE '%&amp;%';