-- =============================================================================
-- seed_event_collections.sql
--
-- 120 events and 6 event collections, built to exercise collection folding on
-- the home feed, the collection page in all three apps, and the Collections
-- editor in the organiser dashboard.
--
-- WHAT YOU SHOULD SEE AFTER RUNNING IT
--
--   109 live events (published, public, in the future), spread over the next
--   ~90 days across 24 South African cities and all 10 categories.
--
--   Collections — on the home feed each one with 2+ live dates collapses into
--   ONE card with the COLLECTION badge:
--
--     1. Afrochild Collab Pop-Up Tour      8 live dates, 8 cities
--          + 1 past (completed) date and 1 draft. Neither counts, so the
--          badge must say 8, not 10.
--     2. LOL Summer Comedy Tour            6 live dates, 6 cities
--     3. Summer Sounds Festival Series     4 live dates, one of them FREE,
--          so the price line must read "From Free" / "Free".
--     4. Ubuntu 10K Run Series             5 live dates, 5 cities
--     5. Winelands Long Table Dinners      ONLY 1 live date + 1 past date.
--          Must NOT fold: the live date shows as a normal event card with no
--          badge. Its collection page still opens from the event page.
--     6. Jozi Jazz Club Residency          3 live dates in ONE city
--          + 1 published_private (invite-only) night that must never appear
--          publicly and must not be counted.
--
--   Standalone events also include 2 completed, 2 published_private,
--   2 drafts and 1 cancelled, so the "live" filters have something to filter.
--
-- ORGANISER LOGINS (dashboard → Collections shows each one's collections):
--   seed.collections1@tickethawk.test   Afrochild Collab
--   seed.collections2@tickethawk.test   LOL Live Comedy
--   seed.collections3@tickethawk.test   Fresh Fields Festivals
--   seed.collections4@tickethawk.test   Ubuntu Runners
--   seed.collections5@tickethawk.test   Mzansi Live Events
--   Password for all five: SeedPassw0rd!
--
-- REQUIRES: migrations up to 000096 (event_collections).
--
-- RUN (from the events-db folder, with the stack up):
--   docker compose exec -T db psql -U devuser -d events_dev < seed_event_collections.sql
--
-- RE-RUNNING: safe. Users, organisers, collections and ticket tiers are only
-- inserted once. Events are UPSERTED on their dates, status and collection,
-- so running it again next week moves every date forward relative to today
-- and the feed is full of live events again. Ticket sales are left alone.
--
-- REMOVE EVERYTHING THIS CREATED:
--   DELETE FROM events            WHERE id::text LIKE 'e7%-5eed-4c01-8000-000000000000';
--   DELETE FROM event_collections WHERE id::text LIKE 'c7%-5eed-4c01-8000-000000000000';
--   DELETE FROM organizers        WHERE id::text LIKE 'a7000000-%';
--   DELETE FROM users             WHERE id::text LIKE 'd7000000-%';
--   (ticket_types cascade with the event. If you made test bookings against
--   these events, delete those bookings first — bookings block event deletes.)
-- =============================================================================

BEGIN;

-- Same slug shape as utils.GenerateSlug: lowercase hyphenated title + the
-- first 8 characters of the id.
CREATE OR REPLACE FUNCTION seed_ec_slug(title TEXT, id UUID) RETURNS TEXT AS $fn$
  SELECT trim(BOTH '-' FROM regexp_replace(lower(title), '[^a-z0-9]+', '-', 'g'))
         || '-' || substr(id::text, 1, 8);
$fn$ LANGUAGE SQL IMMUTABLE;

-- ── Users ────────────────────────────────────────────────────────────────────
-- Password hash is bcrypt (cost 10) of 'SeedPassw0rd!', checked with
-- golang.org/x/crypto/bcrypt, the same library the login handler uses.
INSERT INTO users (id, email, password, first_name, last_name, phone_number, role, email_verified) VALUES
  ('d7000000-0000-0000-0000-000000000001', 'seed.collections1@tickethawk.test', '$2a$10$3AMJbse76qV0sZLoi0GTtep6hKCZmhEJ979xva82hWGIP8NwfL032', 'Seed', 'Collections 1', '+27110000701', 'organizer', TRUE),
  ('d7000000-0000-0000-0000-000000000002', 'seed.collections2@tickethawk.test', '$2a$10$3AMJbse76qV0sZLoi0GTtep6hKCZmhEJ979xva82hWGIP8NwfL032', 'Seed', 'Collections 2', '+27210000702', 'organizer', TRUE),
  ('d7000000-0000-0000-0000-000000000003', 'seed.collections3@tickethawk.test', '$2a$10$3AMJbse76qV0sZLoi0GTtep6hKCZmhEJ979xva82hWGIP8NwfL032', 'Seed', 'Collections 3', '+27210000703', 'organizer', TRUE),
  ('d7000000-0000-0000-0000-000000000004', 'seed.collections4@tickethawk.test', '$2a$10$3AMJbse76qV0sZLoi0GTtep6hKCZmhEJ979xva82hWGIP8NwfL032', 'Seed', 'Collections 4', '+27310000704', 'organizer', TRUE),
  ('d7000000-0000-0000-0000-000000000005', 'seed.collections5@tickethawk.test', '$2a$10$3AMJbse76qV0sZLoi0GTtep6hKCZmhEJ979xva82hWGIP8NwfL032', 'Seed', 'Collections 5', '+27120000705', 'organizer', TRUE)
ON CONFLICT DO NOTHING;

-- ── Organisers ───────────────────────────────────────────────────────────────
-- kyc_level 2 + approved so the public organiser block and page show.
INSERT INTO organizers (id, user_id, slug, business_name, business_email, business_phone,
                        description, logo_url, cover_image_url, website_url,
                        verified, kyc_level, public_profile_enabled, status, commission_rate) VALUES
  ('a7000000-0000-0000-0000-000000000001', 'd7000000-0000-0000-0000-000000000001', 'afrochild-collab', 'Afrochild Collab', 'hello@afrochild.test', '+27110000701',
   'Streetwear, local designers and DJs in one room. We take the pop-up on the road every summer.',
   'https://picsum.photos/seed/c7logo1/400/400', 'https://picsum.photos/seed/c7cover1/1600/700', NULL,
   TRUE, 2, TRUE, 'approved', 10.00),
  ('a7000000-0000-0000-0000-000000000002', 'd7000000-0000-0000-0000-000000000002', 'lol-live-comedy', 'LOL Live Comedy', 'bookings@lollive.test', '+27210000702',
   'Stand-up comedy promoters. Headliners, rising stars and a residency jazz night on the side.',
   'https://picsum.photos/seed/c7logo2/400/400', 'https://picsum.photos/seed/c7cover2/1600/700', NULL,
   TRUE, 2, TRUE, 'approved', 10.00),
  ('a7000000-0000-0000-0000-000000000003', 'd7000000-0000-0000-0000-000000000003', 'fresh-fields-festivals', 'Fresh Fields Festivals', 'info@freshfields.test', '+27210000703',
   'Outdoor music festivals, night markets and long-table dinners across the country.',
   'https://picsum.photos/seed/c7logo3/400/400', 'https://picsum.photos/seed/c7cover3/1600/700', NULL,
   TRUE, 2, TRUE, 'approved', 10.00),
  ('a7000000-0000-0000-0000-000000000004', 'd7000000-0000-0000-0000-000000000004', 'ubuntu-runners', 'Ubuntu Runners', 'run@ubunturunners.test', '+27310000704',
   'Community running club hosting timed 10K, trail and fun runs in every province.',
   'https://picsum.photos/seed/c7logo4/400/400', 'https://picsum.photos/seed/c7cover4/1600/700', NULL,
   TRUE, 2, TRUE, 'approved', 10.00),
  ('a7000000-0000-0000-0000-000000000005', 'd7000000-0000-0000-0000-000000000005', 'mzansi-live-events', 'Mzansi Live Events', 'events@mzansilive.test', '+27120000705',
   'Concerts, conferences, workshops and community days — a bit of everything, all year round.',
   'https://picsum.photos/seed/c7logo5/400/400', 'https://picsum.photos/seed/c7cover5/1600/700', NULL,
   TRUE, 2, TRUE, 'approved', 10.00)
ON CONFLICT DO NOTHING;

-- ── Collections ──────────────────────────────────────────────────────────────
-- organizer_id is the organiser's USER id, same as events.organizer_id.
INSERT INTO event_collections (id, organizer_id, slug, title, description, cover_image_url) VALUES
  ('c7000001-5eed-4c01-8000-000000000000', 'd7000000-0000-0000-0000-000000000001',
   seed_ec_slug('Afrochild Collab Pop-Up Tour', 'c7000001-5eed-4c01-8000-000000000000'),
   'Afrochild Collab Pop-Up Tour',
   'The Afrochild Collab pop-up is on the road: local designers, sneaker swaps, DJs and food stalls, one weekend in every city. Pick your stop.',
   'https://picsum.photos/seed/c7coll1/1080/1620'),
  ('c7000002-5eed-4c01-8000-000000000000', 'd7000000-0000-0000-0000-000000000002',
   seed_ec_slug('LOL Summer Comedy Tour', 'c7000002-5eed-4c01-8000-000000000000'),
   'LOL Summer Comedy Tour',
   'Six cities, one line-up of headliners and rising stars. Ninety minutes of stand-up, no interval.',
   'https://picsum.photos/seed/c7coll2/1080/1620'),
  ('c7000003-5eed-4c01-8000-000000000000', 'd7000000-0000-0000-0000-000000000003',
   seed_ec_slug('Summer Sounds Festival Series', 'c7000003-5eed-4c01-8000-000000000000'),
   'Summer Sounds Festival Series',
   'A one-day outdoor festival that moves city to city all summer. The Mbombela date is a free community edition.',
   'https://picsum.photos/seed/c7coll3/1080/1620'),
  ('c7000004-5eed-4c01-8000-000000000000', 'd7000000-0000-0000-0000-000000000004',
   seed_ec_slug('Ubuntu 10K Run Series', 'c7000004-5eed-4c01-8000-000000000000'),
   'Ubuntu 10K Run Series',
   'Timed, chipped 10K road races in five cities. Run one or collect the whole series medal.',
   'https://picsum.photos/seed/c7coll4/1080/1620'),
  ('c7000005-5eed-4c01-8000-000000000000', 'd7000000-0000-0000-0000-000000000003',
   seed_ec_slug('Winelands Long Table Dinners', 'c7000005-5eed-4c01-8000-000000000000'),
   'Winelands Long Table Dinners',
   'Seasonal long-table suppers in the barrel halls of the Cape winelands.',
   'https://picsum.photos/seed/c7coll5/1080/1620'),
  ('c7000006-5eed-4c01-8000-000000000000', 'd7000000-0000-0000-0000-000000000002',
   seed_ec_slug('Jozi Jazz Club Residency', 'c7000006-5eed-4c01-8000-000000000000'),
   'Jozi Jazz Club Residency',
   'Our weekly residency at The Orbit: a house trio and a different guest horn every week.',
   'https://picsum.photos/seed/c7coll6/1080/1620')
ON CONFLICT DO NOTHING;

-- ── Events ───────────────────────────────────────────────────────────────────
-- Columns in the VALUES list:
--   n      event number → id e7<n, 6 digits>-5eed-4c01-8000-000000000000
--          (the number leads the id so every slug gets its own suffix)
--   coll   collection number (NULL = standalone)
--   org    organiser number (1-5)
--   cat    category id (1 Music, 2 Sports, 3 Arts & Theatre, 4 Food & Drink,
--          5 Business, 6 Community, 7 Technology, 8 Health & Wellness,
--          9 Education, 10 Other)
--   d, h   start = today (SAST) + d days, at h:00 SAST. Negative d = past.
--   dur    length in hours
WITH city(city, lat, lng) AS (VALUES
  ('Johannesburg',     -26.2041, 28.0473), ('Sandton',       -26.1076, 28.0567),
  ('Soweto',           -26.2678, 27.8585), ('Midrand',       -25.9992, 28.1262),
  ('Pretoria',         -25.7479, 28.2293), ('Cape Town',     -33.9249, 18.4241),
  ('Stellenbosch',     -33.9321, 18.8602), ('Franschhoek',   -33.9133, 19.1169),
  ('Paarl',            -33.7342, 18.9621), ('Durban',        -29.8587, 31.0218),
  ('Umhlanga',         -29.7270, 31.0840), ('Pietermaritzburg', -29.6006, 30.3794),
  ('Gqeberha',         -33.9608, 25.6022), ('Jeffreys Bay',  -34.0507, 24.9192),
  ('East London',      -33.0153, 27.9116), ('Bloemfontein',  -29.0852, 26.1596),
  ('Kimberley',        -28.7282, 24.7499), ('Polokwane',     -23.9045, 29.4689),
  ('Mbombela',         -25.4753, 30.9694), ('Rustenburg',    -25.6676, 27.2421),
  ('George',           -33.9630, 22.4617), ('Knysna',        -34.0363, 23.0471),
  ('Hartbeespoort',    -25.7470, 27.8970), ('Clarens',       -28.5160, 28.4210)
),
blurb(cat, txt) AS (VALUES
  (1,  'Live music with a full sound and light rig. Bars open throughout, food trucks on site.'),
  (2,  'Gates open 90 minutes before start. Bring sunscreen and water; spectators welcome.'),
  (3,  'Seated performance. Latecomers are admitted at a suitable break in the show.'),
  (4,  'Tastings, local producers and long tables. Come hungry and bring a cooler bag.'),
  (5,  'Talks, panels and structured networking. Coffee and lunch are included.'),
  (6,  'A community gathering open to everyone. Families and dogs on leads welcome.'),
  (7,  'Talks, demos and hands-on sessions. Bring a laptop; Wi-Fi is provided.'),
  (8,  'Suitable for all levels. Wear comfortable clothes and bring a mat and water.'),
  (9,  'Small-group, hands-on learning. All materials are provided on the day.'),
  (10, 'Something a little different. Check the info section for what to bring.')
),
coll_blurb(coll, txt) AS (VALUES
  (1, 'One stop on the Afrochild Collab Pop-Up Tour: 30+ local designers, a sneaker swap, DJs from midday and food stalls until close.'),
  (2, 'A stop on the LOL Summer Comedy Tour. Ninety minutes of stand-up from four comics, no interval. Strictly 18+.'),
  (3, 'A Summer Sounds Festival Series date: two stages, twelve acts, gates at noon and last act off at 22:30.'),
  (4, 'A race in the Ubuntu 10K Run Series. Chip timed, medal at the finish, seeding pens open 45 minutes before the gun.'),
  (5, 'A Winelands Long Table Dinner: five courses paired with the estate''s wines, seated at one long table in the barrel hall.'),
  (6, 'Jozi Jazz Club Residency night: the house trio plus a guest horn, two sets.')
),
v(n, coll, org, cat, title, venue, city, tags, price, cap, sold, d, h, dur, status, feat, views) AS (VALUES
  -- ── Collection 1: Afrochild Collab Pop-Up Tour (8 live + 1 past + 1 draft) ──
  (  1, 1, 1,  6, 'Afrochild Pop-Up: Johannesburg',   'The Sheds @ 1Fox',             'Johannesburg', ARRAY['popup','fashion','market','music'], 150.00, 1200,  340,   5, 11, 10, 'published', TRUE,  9800),
  (  2, 1, 1,  6, 'Afrochild Pop-Up: Pretoria',       'Pretoria Showgrounds',         'Pretoria',     ARRAY['popup','fashion','market','music'], 120.00,  900,  210,   9, 11, 10, 'published', FALSE, 4100),
  (  3, 1, 1,  6, 'Afrochild Pop-Up: Durban',         'Station Drive Precinct',       'Durban',       ARRAY['popup','fashion','market','music'], 120.00, 1000,  260,  12, 11, 10, 'published', FALSE, 3900),
  (  4, 1, 1,  6, 'Afrochild Pop-Up: Cape Town',      'The Old Biscuit Mill',         'Cape Town',    ARRAY['popup','fashion','market','music'], 150.00, 1200,  480,  16, 11, 10, 'published', FALSE, 5200),
  (  5, 1, 1,  6, 'Afrochild Pop-Up: Gqeberha',       'Baakens Valley Precinct',      'Gqeberha',     ARRAY['popup','fashion','market','music'], 100.00,  600,   90,  19, 11, 10, 'published', FALSE, 1700),
  (  6, 1, 1,  6, 'Afrochild Pop-Up: Bloemfontein',   'Loch Logan Waterfront',        'Bloemfontein', ARRAY['popup','fashion','market','music'], 100.00,  600,   75,  23, 11, 10, 'published', FALSE, 1500),
  (  7, 1, 1,  6, 'Afrochild Pop-Up: East London',    'Quigney Beachfront',           'East London',  ARRAY['popup','fashion','market','music'], 100.00,  500,   60,  26, 11, 10, 'published', FALSE, 1200),
  (  8, 1, 1,  6, 'Afrochild Pop-Up: Polokwane',      'Peter Mokaba Precinct',        'Polokwane',    ARRAY['popup','fashion','market','music'], 100.00,  600,   40,  30, 11, 10, 'published', FALSE,  900),
  (  9, 1, 1,  6, 'Afrochild Pop-Up: Soweto',         'Vilakazi Street Precinct',     'Soweto',       ARRAY['popup','fashion','market','music'], 100.00,  800,  780, -10, 11, 10, 'completed', FALSE, 6400),
  ( 10, 1, 1,  6, 'Afrochild Pop-Up: Kimberley',      'Big Hole Precinct',            'Kimberley',    ARRAY['popup','fashion','market','music'], 100.00,  500,    0,  40, 11, 10, 'draft',     FALSE,    0),

  -- ── Collection 2: LOL Summer Comedy Tour (6 live) ──
  ( 11, 2, 2,  3, 'LOL Summer Comedy Tour: Cape Town',        'Baxter Theatre',            'Cape Town',        ARRAY['comedy','standup','theatre'], 220.00, 650, 410,  7, 19, 3, 'published', FALSE, 4300),
  ( 12, 2, 2,  3, 'LOL Summer Comedy Tour: Stellenbosch',     'Oude Libertas Amphitheatre','Stellenbosch',     ARRAY['comedy','standup','theatre'], 200.00, 500, 180, 10, 19, 3, 'published', FALSE, 2100),
  ( 13, 2, 2,  3, 'LOL Summer Comedy Tour: George',           'George Arts Theatre',       'George',           ARRAY['comedy','standup','theatre'], 180.00, 300,  70, 14, 19, 3, 'published', FALSE,  800),
  ( 14, 2, 2,  3, 'LOL Summer Comedy Tour: Durban',           'The Playhouse',             'Durban',           ARRAY['comedy','standup','theatre'], 220.00, 900, 300, 21, 19, 3, 'published', FALSE, 2600),
  ( 15, 2, 2,  3, 'LOL Summer Comedy Tour: Pietermaritzburg', 'Hexagon Theatre',           'Pietermaritzburg', ARRAY['comedy','standup','theatre'], 180.00, 400,  60, 24, 19, 3, 'published', FALSE,  700),
  ( 16, 2, 2,  3, 'LOL Summer Comedy Tour: Johannesburg',     'Joburg Theatre',            'Johannesburg',     ARRAY['comedy','standup','theatre'], 250.00,1000, 520, 28, 19, 3, 'published', FALSE, 5100),

  -- ── Collection 3: Summer Sounds Festival Series (4 live, one free) ──
  ( 17, 3, 3,  1, 'Summer Sounds: Cape Town',  'Grand Parade',             'Cape Town',    ARRAY['festival','music','outdoor'], 450.00, 8000, 3100, 18, 12, 11, 'published', FALSE, 8800),
  ( 18, 3, 3,  1, 'Summer Sounds: Durban',     'North Beach Amphitheatre', 'Durban',       ARRAY['festival','music','outdoor'], 395.00, 6000, 1900, 32, 12, 11, 'published', FALSE, 6100),
  ( 19, 3, 3,  1, 'Summer Sounds: Johannesburg','Zoo Lake Gardens',        'Johannesburg', ARRAY['festival','music','outdoor'], 450.00, 9000, 2700, 46, 12, 11, 'published', FALSE, 7300),
  ( 20, 3, 3,  1, 'Summer Sounds: Mbombela (Free Community Edition)', 'Mbombela Stadium Grounds', 'Mbombela', ARRAY['festival','music','outdoor','free'], 0.00, 5000, 2100, 60, 12, 11, 'published', FALSE, 3300),

  -- ── Collection 4: Ubuntu 10K Run Series (5 live) ──
  ( 21, 4, 4,  2, 'Ubuntu 10K: Cape Town',    'Green Point Urban Park',  'Cape Town',    ARRAY['running','10k','fitness'], 150.00, 2000, 860,  8, 6, 4, 'published', FALSE, 2900),
  ( 22, 4, 4,  2, 'Ubuntu 10K: Durban',       'Moses Mabhida Stadium',   'Durban',       ARRAY['running','10k','fitness'], 150.00, 2000, 640, 15, 6, 4, 'published', FALSE, 2200),
  ( 23, 4, 4,  2, 'Ubuntu 10K: Johannesburg', 'Emmarentia Dam',          'Johannesburg', ARRAY['running','10k','fitness'], 150.00, 2500, 910, 22, 6, 4, 'published', FALSE, 3100),
  ( 24, 4, 4,  2, 'Ubuntu 10K: Pretoria',     'Union Buildings Gardens', 'Pretoria',     ARRAY['running','10k','fitness'], 120.00, 2000, 420, 29, 6, 4, 'published', FALSE, 1600),
  ( 25, 4, 4,  2, 'Ubuntu 10K: Gqeberha',     'Hobie Beach',             'Gqeberha',     ARRAY['running','10k','fitness'], 120.00, 1500, 180, 36, 6, 4, 'published', FALSE,  900),

  -- ── Collection 5: Winelands Long Table Dinners (1 live + 1 past → must NOT fold) ──
  ( 26, 5, 3,  4, 'Long Table Dinner: Franschhoek', 'Franschhoek Estate Barrel Hall', 'Franschhoek', ARRAY['food','wine','dinner'], 850.00, 80, 52,  20, 18, 4, 'published', FALSE, 1400),
  ( 27, 5, 3,  4, 'Long Table Dinner: Paarl',       'Paarl Estate Cellar',            'Paarl',       ARRAY['food','wine','dinner'], 850.00, 80, 80, -20, 18, 4, 'completed', FALSE, 2000),

  -- ── Collection 6: Jozi Jazz Club Residency (3 live in one city + 1 private) ──
  ( 28, 6, 2,  1, 'Jozi Jazz Residency: Week 1',            'The Orbit Jazz Club', 'Johannesburg', ARRAY['jazz','music','live'], 180.00, 220, 140,  6, 20, 4, 'published',         FALSE, 1900),
  ( 29, 6, 2,  1, 'Jozi Jazz Residency: Week 2',            'The Orbit Jazz Club', 'Johannesburg', ARRAY['jazz','music','live'], 180.00, 220,  90, 13, 20, 4, 'published',         FALSE, 1100),
  ( 30, 6, 2,  1, 'Jozi Jazz Residency: Week 3',            'The Orbit Jazz Club', 'Johannesburg', ARRAY['jazz','music','live'], 180.00, 220,  40, 20, 20, 4, 'published',         FALSE,  600),
  ( 31, 6, 2,  1, 'Jozi Jazz Residency: Members Night',     'The Orbit Jazz Club', 'Johannesburg', ARRAY['jazz','music','live'],   0.00, 120,  35, 27, 20, 4, 'published_private', FALSE,   40),

  -- ── Standalone: Music ──
  ( 32, NULL, 5,  1, 'Amapiano Sundays: Rooftop Edition', 'Rooftop at 44 Stanley',     'Johannesburg', ARRAY['amapiano','music','nightlife'], 200.00,  400,  120,  3, 15,  7, 'published', FALSE, 3600),
  ( 33, NULL, 5,  1, 'Deep House Sunset Sessions',        'Café Caprice',              'Cape Town',    ARRAY['house','music','sunset'],       250.00,  300,   90,  4, 16,  6, 'published', FALSE, 2800),
  ( 34, NULL, 5,  1, 'Gqom Takeover',                     'Florida Road Strip',        'Durban',       ARRAY['gqom','music','nightlife'],     150.00,  500,  210, 11, 20,  6, 'published', FALSE, 2400),
  ( 35, NULL, 5,  1, 'Kwaito Throwback Night',            'Newtown Music Hall',        'Johannesburg', ARRAY['kwaito','music','retro'],       180.00,  800,  300, 17, 20,  5, 'published', FALSE, 3100),
  ( 36, NULL, 5,  1, 'Maskandi Heritage Concert',         'Durban ICC Arena',          'Durban',       ARRAY['maskandi','music','heritage'],  220.00, 2500,  800, 24, 17,  5, 'published', FALSE, 2700),
  ( 37, NULL, 5,  1, 'Gospel Praise Night',               'Ellis Park Arena',          'Johannesburg', ARRAY['gospel','music','choir'],       250.00, 5000, 1900, 31, 18,  4, 'published', TRUE,  7400),
  ( 38, NULL, 5,  1, 'Indie Rock on the Lawn',            'Kirstenbosch Gardens',      'Cape Town',    ARRAY['rock','music','outdoor'],       320.00, 3000, 2900, 38, 16,  4, 'published', FALSE, 6200),
  ( 39, NULL, 5,  1, 'Afro Jazz Brunch',                  'Sakhumzi Restaurant',       'Soweto',       ARRAY['jazz','music','brunch'],        280.00,  120,  120,  2, 10,  4, 'published', FALSE, 4800),
  ( 40, NULL, 5,  1, 'Classical Strings at the Castle',   'Castle of Good Hope',       'Cape Town',    ARRAY['classical','music','strings'],  300.00,  400,  110, 45, 18,  2, 'published', FALSE, 1300),
  ( 41, NULL, 1,  1, 'Hip Hop Block Party',               'Maboneng Precinct',         'Johannesburg', ARRAY['hiphop','music','street'],      120.00, 1500,  600, 52, 13,  9, 'published', FALSE, 2500),
  ( 42, NULL, 5,  1, 'Reggae by the Sea',                 'Muizenberg Pavilion',       'Cape Town',    ARRAY['reggae','music','beach'],       150.00, 1000,  200, 59, 14,  6, 'published', FALSE, 1100),
  ( 43, NULL, 5,  1, 'Open Mic Acoustic Night',           'Arcade Empire',             'Pretoria',     ARRAY['acoustic','music','openmic'],     0.00,  150,   60,  1, 19,  3, 'published', FALSE,  900),
  ( 44, NULL, 1,  1, 'Electronic Warehouse Rave',         'Durban Warehouse District', 'Durban',       ARRAY['electronic','music','rave'],    300.00, 1200,  450, 66, 21,  7, 'published', FALSE, 1800),
  ( 45, NULL, 5,  1, 'Choir Festival Finals',             'Polokwane City Hall',       'Polokwane',    ARRAY['choir','music','competition'],   80.00,  900,  300, 73, 14,  4, 'published', FALSE,  700),
  ( 46, NULL, 5,  1, 'Afrobeats Summer Bash',             'Bloem Showgrounds',         'Bloemfontein', ARRAY['afrobeats','music','summer'],   180.00, 2000,  700, 80, 15,  8, 'published', FALSE, 1200),

  -- ── Standalone: Sports ──
  ( 47, NULL, 4,  2, 'Sunday Social League Final',        'Dobsonville Stadium',        'Soweto',        ARRAY['football','final','community'],  50.00, 8000, 3200,  4, 14, 3, 'published', FALSE, 3300),
  ( 48, NULL, 5,  2, 'Pro Boxing Fight Night',            'Gallagher Convention Centre','Midrand',       ARRAY['boxing','fight','sport'],       450.00, 3000, 1100, 12, 18, 5, 'published', TRUE,  6900),
  ( 49, NULL, 4,  2, 'Comrades Training Run',             'Pietermaritzburg City Hall', 'Pietermaritzburg', ARRAY['running','training','free'],  0.00,  500,  240,  6,  5, 5, 'published', FALSE, 1400),
  ( 50, NULL, 4,  2, 'Beach Volleyball Open',             'Camps Bay Beach',            'Cape Town',     ARRAY['volleyball','beach','sport'],    80.00,  600,   90, 18,  9, 8, 'published', FALSE,  800),
  ( 51, NULL, 4,  2, 'Netball Inter-Provincial',          'Ruimsig Stadium',            'Johannesburg',  ARRAY['netball','sport'],               60.00, 2000,  400, 25, 10, 7, 'published', FALSE,  950),
  ( 52, NULL, 4,  2, 'Jonkershoek Mountain Bike Enduro',  'Jonkershoek Nature Reserve', 'Stellenbosch',  ARRAY['mtb','cycling','outdoor'],      350.00,  400,  180, 33,  7, 6, 'published', FALSE, 1000),
  ( 53, NULL, 5,  2, 'J-Bay Surf Classic',                'Supertubes',                 'Jeffreys Bay',  ARRAY['surf','beach','free'],            0.00, 1500,    0, 40,  7, 9, 'published', FALSE, 2100),
  ( 54, NULL, 5,  2, 'Rugby Sevens Fan Day',              'Cape Town Stadium',          'Cape Town',     ARRAY['rugby','sevens','sport'],       250.00,20000, 8700, 47, 11, 8, 'published', TRUE,  9100),
  ( 55, NULL, 5,  2, 'T20 Night Derby',                   'Wanderers Stadium',          'Johannesburg',  ARRAY['cricket','t20','sport'],        180.00,15000, 9300, 54, 17, 4, 'published', FALSE, 8200),
  ( 56, NULL, 5,  2, 'Golf Charity Day',                  'Durban Country Club',        'Durban',        ARRAY['golf','charity','sport'],      1200.00,  144,   98, 61,  7, 8, 'published', FALSE,  600),
  ( 57, NULL, 4,  2, 'Magaliesberg 21K Trail Run',        'Hartbeespoort Dam Trailhead','Hartbeespoort', ARRAY['trail','running','outdoor'],    280.00,  800,  300, 68,  6, 6, 'published', FALSE, 1100),
  ( 58, NULL, 4,  2, 'Durban Harbour Kayak Race',         'Wilson''s Wharf',            'Durban',        ARRAY['kayak','water','sport'],        150.00,  200,   30, 75,  8, 5, 'published', FALSE,  400),

  -- ── Standalone: Arts & Theatre ──
  ( 59, NULL, 2,  3, 'Shakespeare in the Park',           'Maynardville Open-Air Theatre','Cape Town',   ARRAY['theatre','shakespeare','outdoor'], 220.00,  700,  250,  9, 19, 3, 'published', FALSE, 1700),
  ( 60, NULL, 5,  3, 'Contemporary Dance Showcase',       'The Dance Factory',          'Johannesburg',  ARRAY['dance','contemporary','arts'],  150.00,  350,   80, 16, 19, 2, 'published', FALSE,  900),
  ( 61, NULL, 5,  3, 'Spoken Word Slam',                  'Soweto Theatre',             'Soweto',        ARRAY['poetry','spokenword','arts'],    80.00,  400,  150,  5, 18, 3, 'published', FALSE, 1300),
  ( 62, NULL, 5,  3, 'First Thursdays Gallery Walk',      'Bree Street Galleries',      'Cape Town',     ARRAY['art','gallery','free'],           0.00, 2000,    0,  1, 17, 5, 'published', FALSE, 2600),
  ( 63, NULL, 5,  3, 'Kofifi: A Sophiatown Musical',      'Teatro at Montecasino',      'Johannesburg',  ARRAY['musical','theatre','heritage'], 350.00, 1800, 1200, 20, 19, 3, 'published', TRUE,  8300),
  ( 64, NULL, 5,  3, 'Photography Exhibition Opening',    'Durban Art Gallery',         'Durban',        ARRAY['photography','exhibition','free'], 0.00,  300,    0, 27, 17, 3, 'published', FALSE,  700),
  ( 65, NULL, 2,  3, 'Stand-Up Rising Stars',             'Parker''s Comedy & Jive',    'Johannesburg',  ARRAY['comedy','standup'],             150.00,  300,  260,  3, 20, 2, 'published', FALSE, 2300),
  ( 66, NULL, 5,  3, 'Puppet Theatre for Kids',           'Artscape Theatre',           'Cape Town',     ARRAY['kids','puppets','family'],      100.00,  500,  130, 34, 10, 2, 'published', FALSE,  600),
  ( 67, NULL, 5,  3, 'Ballet Gala Night',                 'South African State Theatre','Pretoria',      ARRAY['ballet','dance','gala'],        400.00, 1300,  500, 41, 19, 3, 'published', FALSE, 1800),
  ( 68, NULL, 5,  3, 'Film Festival Opening Night',       'The Labia Theatre',          'Cape Town',     ARRAY['film','festival','cinema'],     120.00,  150,  150, 48, 19, 3, 'published', FALSE, 2900),
  ( 69, NULL, 5,  3, 'Clarens Pottery Market',            'Clarens Village Square',     'Clarens',       ARRAY['pottery','market','free'],        0.00, 1000,    0, 55,  9, 7, 'published', FALSE,  500),

  -- ── Standalone: Food & Drink ──
  ( 70, NULL, 3,  4, 'Night Market Fridays',              'Victoria Yards',             'Johannesburg',  ARRAY['market','food','night'],         50.00, 2500,  700,  2, 17, 6, 'published', FALSE, 3800),
  ( 71, NULL, 3,  4, 'Craft Beer Festival',               'Durban Botanic Gardens',     'Durban',        ARRAY['beer','craft','festival'],      200.00, 3000, 1400, 11, 12, 8, 'published', FALSE, 3400),
  ( 72, NULL, 3,  4, 'Braai Masters Championship',        'Bloem Showgrounds',          'Bloemfontein',  ARRAY['braai','competition','food'],   120.00, 2500,  500, 19, 10, 8, 'published', FALSE, 1500),
  ( 73, NULL, 3,  4, 'Gin & Tonic Garden Party',          'Irene Dairy Farm',           'Pretoria',      ARRAY['gin','garden','party'],         280.00,  800,  300, 26, 12, 6, 'published', FALSE, 1900),
  ( 74, NULL, 3,  4, 'Kalk Bay Seafood Festival',         'Kalk Bay Harbour',           'Cape Town',     ARRAY['seafood','festival','harbour'], 180.00, 2000,  800, 35, 11, 7, 'published', FALSE, 2200),
  ( 75, NULL, 3,  4, 'Street Food Sunday',                'Braamfontein Market',        'Johannesburg',  ARRAY['streetfood','market','free'],     0.00, 3000,    0,  7, 10, 6, 'published', FALSE, 2000),
  ( 76, NULL, 3,  4, 'Cheese & Wine Tasting',             'Stellenbosch Wine Hub',      'Stellenbosch',  ARRAY['wine','cheese','tasting'],      350.00,  150,   90, 42, 15, 3, 'published', FALSE,  800),
  ( 77, NULL, 3,  4, 'Vegan Food Fair',                   'Umhlanga Promenade',         'Umhlanga',      ARRAY['vegan','food','fair'],           60.00, 1200,  200, 49, 10, 6, 'published', FALSE,  700),
  ( 78, NULL, 5,  4, 'Chocolate Making Workshop',         'Bryanston Organic Market',   'Sandton',       ARRAY['chocolate','workshop','food'],  450.00,   30,   28, 56, 10, 3, 'published', FALSE,  500),
  ( 79, NULL, 3,  4, 'Knysna Oyster Festival',            'Knysna Waterfront',          'Knysna',        ARRAY['oysters','festival','seafood'], 250.00, 4000, 1300, 63, 11, 8, 'published', FALSE, 2600),

  -- ── Standalone: Business ──
  ( 80, NULL, 5,  5, 'Startup Pitch Night',               'Workshop17 Sandton',         'Sandton',       ARRAY['startup','pitch','networking'], 150.00,  200,  160,  8, 18, 3, 'published', FALSE, 1600),
  ( 81, NULL, 5,  5, 'Women in Leadership Summit',        'Sandton Convention Centre',  'Sandton',       ARRAY['leadership','women','summit'], 1500.00,  800,  450, 22,  8, 9, 'published', TRUE,  5400),
  ( 82, NULL, 5,  5, 'SME Funding Expo',                  'Durban ICC',                 'Durban',        ARRAY['sme','funding','expo','free'],    0.00, 1500,  600, 36,  9, 8, 'published', FALSE, 1800),
  ( 83, NULL, 5,  5, 'Property Investment Seminar',       'CTICC',                      'Cape Town',     ARRAY['property','investment'],        650.00,  500,  180, 50,  9, 7, 'published', FALSE, 1100),
  ( 84, NULL, 5,  5, 'Township Entrepreneurs Breakfast',  'Soweto Hotel',               'Soweto',        ARRAY['entrepreneurs','breakfast'],    120.00,  150,   90, 13,  7, 3, 'published', FALSE,  600),
  ( 85, NULL, 5,  5, 'Pretoria Business Awards',          'CSIR ICC',                   'Pretoria',      ARRAY['awards','business','gala'],     950.00,  600,  420, 71, 18, 5, 'published', FALSE,  900),

  -- ── Standalone: Community ──
  ( 86, NULL, 5,  6, 'Blouberg Beach Clean-Up',           'Blouberg Beach',             'Cape Town',     ARRAY['cleanup','beach','free'],         0.00,  300,    0,  3,  8, 3, 'published', FALSE,  700),
  ( 87, NULL, 5,  6, 'Spring Picnic in the Park',         'Delta Park',                 'Johannesburg',  ARRAY['picnic','family','free'],         0.00, 1000,    0, 10, 11, 5, 'published', FALSE, 1200),
  ( 88, NULL, 4,  6, 'Mamelodi Family Fun Run',           'Mamelodi Community Grounds', 'Pretoria',      ARRAY['funrun','family','community'],   30.00, 1000,  250, 17,  8, 6, 'published', FALSE,  800),
  ( 89, NULL, 5,  6, 'Book Swap & Story Time',            'Bloemfontein Public Library','Bloemfontein',  ARRAY['books','kids','free'],            0.00,   80,    0, 24, 10, 3, 'published', FALSE,  300),
  ( 90, NULL, 5,  6, 'Charity Car Wash & Braai',          'Rustenburg Civic Centre',    'Rustenburg',    ARRAY['charity','braai','community'],   50.00,  300,   60, 31,  9, 6, 'published', FALSE,  400),
  ( 91, NULL, 5,  6, 'Long Street Year-End Party',        'Long Street',                'Cape Town',     ARRAY['street','party','yearend'],     100.00, 5000, 1800, 77, 16, 8, 'published', FALSE, 3700),

  -- ── Standalone: Technology ──
  ( 92, NULL, 5,  7, 'AI Builders Meetup',                'Tshimologong Precinct',      'Johannesburg',  ARRAY['ai','meetup','free'],             0.00,  250,  180,  2, 18,  3, 'published', FALSE, 2100),
  ( 93, NULL, 5,  7, 'Fintech for Everyone Hackathon',    'Workshop17 Watershed',       'Cape Town',     ARRAY['hackathon','fintech'],          100.00,  300,  210, 14,  9, 33, 'published', FALSE, 1500),
  ( 94, NULL, 5,  7, 'Cloud & DevOps Day',                'Durban Tech Hub',            'Durban',        ARRAY['cloud','devops','conference'],  450.00,  400,  120, 28,  8,  9, 'published', FALSE,  800),
  ( 95, NULL, 5,  7, 'Women Who Code Workshop',           'The Innovation Hub',         'Pretoria',      ARRAY['coding','women','free'],          0.00,   80,   80,  9,  9,  6, 'published', FALSE, 1000),
  ( 96, NULL, 5,  7, 'Mobile Dev Conference',             'Sandton Convention Centre',  'Sandton',       ARRAY['mobile','dev','conference'],   1800.00, 1200,  500, 58,  8, 10, 'published', TRUE,  4700),
  ( 97, NULL, 5,  7, 'Gaming Expo',                       'Ticketpro Dome',             'Johannesburg',  ARRAY['gaming','expo','esports'],      220.00, 6000, 2400, 65, 10,  9, 'published', FALSE, 5600),

  -- ── Standalone: Health & Wellness ──
  ( 98, NULL, 4,  8, 'Sunrise Yoga on the Beach',         'Clifton 4th Beach',          'Cape Town',     ARRAY['yoga','beach','sunrise'],       120.00,   80,   60,  1,  6, 2, 'published', FALSE, 1100),
  ( 99, NULL, 4,  8, 'Wellness Retreat Day',              'Hartbeespoort Wellness Lodge','Hartbeespoort',ARRAY['wellness','retreat','spa'],     950.00,   60,   38, 23,  8, 9, 'published', FALSE,  700),
  (100, NULL, 4,  8, 'Breathwork & Sound Bath',           'Durban Botanic Gardens',     'Durban',        ARRAY['breathwork','soundbath'],       200.00,   60,   25, 12, 17, 2, 'published', FALSE,  500),
  (101, NULL, 4,  8, 'HIIT in the Park',                  'Zoo Lake Gardens',           'Johannesburg',  ARRAY['hiit','fitness','free'],          0.00,  200,    0,  4,  7, 1, 'published', FALSE,  600),
  (102, NULL, 4,  8, 'Mental Health Awareness Walk',      'Union Buildings Gardens',    'Pretoria',      ARRAY['walk','mentalhealth','free'],     0.00, 2000,    0, 37,  8, 3, 'published', FALSE,  900),
  (103, NULL, 4,  8, 'Pilates Masterclass',               'Umhlanga Wellness Studio',   'Umhlanga',      ARRAY['pilates','fitness','class'],    250.00,   40,   40, 44,  9, 2, 'published', FALSE,  500),

  -- ── Standalone: Education ──
  (104, NULL, 5,  9, 'Photography Basics Workshop',       'Maboneng Studios',           'Johannesburg',  ARRAY['photography','workshop'],       400.00,   25,   12,  6, 10, 5, 'published', FALSE,  600),
  (105, NULL, 5,  9, 'Matric Career Expo',                'Polokwane Showgrounds',      'Polokwane',     ARRAY['careers','students','free'],      0.00, 3000,    0, 15,  9, 7, 'published', FALSE, 1300),
  (106, NULL, 5,  9, 'Coding for Kids Holiday Camp',      'Sci-Bono Discovery Centre',  'Johannesburg',  ARRAY['kids','coding','camp'],         300.00,   60,   45, 57,  9, 6, 'published', FALSE,  700),
  (107, NULL, 5,  9, 'Financial Literacy Masterclass',    'East London ICC',            'East London',   ARRAY['finance','money','class'],      150.00,  250,   60, 30,  9, 4, 'published', FALSE,  500),
  (108, NULL, 5,  9, 'Creative Writing Weekend',          'Nelson Mandela University',  'Gqeberha',      ARRAY['writing','workshop'],           500.00,   40,   18, 43,  9, 7, 'published', FALSE,  400),
  (109, NULL, 5,  9, 'Constitution Hill History Walk',    'Constitution Hill',          'Johannesburg',  ARRAY['history','walkingtour'],         90.00,   40,   22,  5, 10, 3, 'published', FALSE,  800),

  -- ── Standalone: Other ──
  (110, NULL, 5, 10, 'Classic Car Show',                  'Kyalami Grand Prix Circuit', 'Midrand',       ARRAY['cars','classic','show'],        150.00, 5000, 1200, 39,  9, 8, 'published', FALSE, 2400),
  (111, NULL, 5, 10, 'Pet Adoption Day',                  'Durban SPCA Grounds',        'Durban',        ARRAY['pets','adoption','free'],         0.00,  500,    0,  8, 10, 5, 'published', FALSE,  900),
  (112, NULL, 5, 10, 'Rosebank Rooftop Flea Market',      'Rosebank Rooftop Market',    'Johannesburg',  ARRAY['flea','market','free'],           0.00, 3000,    0, 11,  9, 6, 'published', FALSE, 1600),
  (113, NULL, 5, 10, 'Haunted Maze Nights',               'Gold Reef City',             'Johannesburg',  ARRAY['halloween','maze','family'],    180.00, 2000,  900, 16, 18, 5, 'published', FALSE, 3000),

  -- ── Standalone: not live (past / private / draft / cancelled) ──
  (114, NULL, 5,  1, 'Summer Jazz Picnic',                'Emmarentia Dam',             'Johannesburg',  ARRAY['jazz','picnic'],                200.00, 1500, 1420, -14, 12, 6, 'completed',         FALSE, 5100),
  (115, NULL, 3,  4, 'Durban Food Truck Friday',          'Durban Point Waterfront',    'Durban',        ARRAY['foodtruck','market'],            50.00, 1000,  720,  -6, 17, 5, 'completed',         FALSE, 2300),
  (116, NULL, 5,  5, 'Tech Leaders Private Dinner',       'The Saxon Hotel',            'Sandton',       ARRAY['dinner','invite'],             1200.00,   40,   22,  19, 19, 3, 'published_private', FALSE,   30),
  (117, NULL, 3,  4, 'Secret Garden Supper Club',         'Private Garden, Constantia', 'Cape Town',     ARRAY['supperclub','invite'],          650.00,   30,   12,  27, 18, 4, 'published_private', FALSE,   20),
  (118, NULL, 5,  1, 'New Year''s Eve Countdown',         'V&A Waterfront Amphitheatre','Cape Town',     ARRAY['nye','music','party'],          300.00, 5000,    0,  77, 20, 6, 'draft',             FALSE,    0),
  (119, NULL, 4,  2, 'Ubuntu Trail Ultra 50K',            'Midmar Dam',                 'Pietermaritzburg', ARRAY['ultra','trail','running'],  480.00,  600,    0,  85,  5,10, 'draft',             FALSE,    0),
  (120, NULL, 5, 10, 'Open-Air Cinema Night',             'Delta Park',                 'Johannesburg',  ARRAY['cinema','outdoor'],              80.00,  400,    0,  21, 19, 3, 'cancelled',         FALSE,  300)
),
r AS (
  SELECT v.*,
         ('e7' || lpad(v.n::text, 6, '0') || '-5eed-4c01-8000-000000000000')::uuid                AS eid,
         CASE WHEN v.coll IS NULL THEN NULL
              ELSE ('c7' || lpad(v.coll::text, 6, '0') || '-5eed-4c01-8000-000000000000')::uuid END AS cid,
         ('d7000000-0000-0000-0000-' || lpad(v.org::text, 12, '0'))::uuid               AS uid,
         -- h:00 local time in Johannesburg, d days from today.
         ((date_trunc('day', NOW() AT TIME ZONE 'Africa/Johannesburg')
             + make_interval(days => v.d, hours => v.h)) AT TIME ZONE 'Africa/Johannesburg') AS st
  FROM v
)
INSERT INTO events (
  id, slug, title, description, cover_image_url, banner_image_url,
  category_id, organizer_id, venue_name, address, city, timezone,
  latitude, longitude, tags, price, currency, max_capacity, tickets_sold,
  status, visibility, start_time, end_time, expires_at, published_at,
  is_featured, views, event_collection_id, created_at,
  followers_notified_at, lineup_notified_at, lineup_announced_snapshot
)
SELECT
  r.eid,
  seed_ec_slug(r.title, r.eid),
  r.title,
  COALESCE(cb.txt, b.txt) || ' ' || r.title || ' at ' || r.venue || ', ' || r.city || '.',
  'https://picsum.photos/seed/c7ev' || r.n || '/1080/1620',
  'https://picsum.photos/seed/c7evb' || r.n || '/1920/900',
  r.cat, r.uid, r.venue, r.venue || ', ' || r.city, r.city, 'Africa/Johannesburg',
  -- Small per-event offset so pins in the same city don't stack exactly.
  round((c.lat + ((r.n % 7) - 3) * 0.004)::numeric, 6),
  round((c.lng + ((r.n % 5) - 2) * 0.004)::numeric, 6),
  r.tags, r.price, 'ZAR', r.cap, r.sold,
  r.status::event_status,
  CASE WHEN r.status = 'published_private' THEN 'private_link' ELSE 'public' END,
  r.st,
  r.st + make_interval(hours => r.dur),
  r.st + make_interval(hours => r.dur) + INTERVAL '14 days',
  CASE WHEN r.status = 'draft' THEN NULL
       ELSE LEAST(NOW(), r.st) - make_interval(days => (r.n % 25) + 3) END,
  r.feat, r.views, r.cid,
  -- Spread creation over the last month so "Recently added" has an order.
  NOW() - make_interval(days => (r.n * 7) % 30, hours => r.n % 24),
  -- Mark as already announced so the follower / line-up workers don't send
  -- 120 notifications the first minute the API runs.
  CASE WHEN r.status = 'draft' THEN NULL ELSE NOW() END,
  NOW(),
  '{}'::jsonb
FROM r
JOIN city  c  ON c.city = r.city
JOIN blurb b  ON b.cat  = r.cat
LEFT JOIN coll_blurb cb ON cb.coll = r.coll
ON CONFLICT (id) DO UPDATE SET
  status              = EXCLUDED.status,
  visibility          = EXCLUDED.visibility,
  start_time          = EXCLUDED.start_time,
  end_time            = EXCLUDED.end_time,
  expires_at          = EXCLUDED.expires_at,
  published_at        = EXCLUDED.published_at,
  event_collection_id = EXCLUDED.event_collection_id;

-- ── Ticket tiers ─────────────────────────────────────────────────────────────
-- General Admission for every event; a VIP tier (2x price, 10% of capacity)
-- on paid events of 100+ seats. Tier sales add up to the event's tickets_sold.
-- events.price is kept in sync by trg_sync_event_price (cheapest visible tier),
-- which is the GA price, so nothing shifts.
WITH e AS (
  SELECT id, price, max_capacity AS cap, tickets_sold AS sold,
         CASE WHEN price > 0 AND max_capacity >= 100 THEN max_capacity / 10 ELSE 0 END AS vip_cap
  FROM events
  WHERE id::text LIKE 'e7%-5eed-4c01-8000-000000000000'
    AND NOT EXISTS (SELECT 1 FROM ticket_types t WHERE t.event_id = events.id)
)
INSERT INTO ticket_types (event_id, name, description, price, capacity, tickets_sold, reserved_count, sort_order, is_visible)
SELECT id, 'General Admission',
       CASE WHEN price = 0 THEN 'Free entry — registration required.' ELSE 'Standard entry to the event.' END,
       price, cap - vip_cap, LEAST(sold, cap - vip_cap), 0, 0, TRUE
FROM e
UNION ALL
SELECT id, 'VIP', 'Fast-track entry, reserved area and a welcome drink.',
       price * 2, vip_cap, sold - LEAST(sold, cap - vip_cap), 0, 1, TRUE
FROM e
WHERE vip_cap > 0;

DROP FUNCTION IF EXISTS seed_ec_slug(TEXT, UUID);

COMMIT;

-- ── Verify ───────────────────────────────────────────────────────────────────
-- live_dates is what the home-feed badge counts. Collection 5 must show 1
-- (so it does NOT fold); every other collection 3+.
SELECT c.title,
       count(e.id)                                                             AS all_dates,
       count(e.id) FILTER (WHERE e.status = 'published' AND e.end_time >= NOW()) AS live_dates,
       CASE WHEN count(e.id) FILTER (WHERE e.status = 'published' AND e.end_time >= NOW()) >= 2
            THEN 'folds into one card' ELSE 'shows as normal event' END        AS on_home_feed
FROM event_collections c
LEFT JOIN events e ON e.event_collection_id = c.id
WHERE c.id::text LIKE 'c7%-5eed-4c01-8000-000000000000'
GROUP BY c.id, c.title
ORDER BY c.title;

SELECT status, visibility, count(*) AS events,
       count(*) FILTER (WHERE end_time >= NOW()) AS still_upcoming
FROM events WHERE id::text LIKE 'e7%-5eed-4c01-8000-000000000000'
GROUP BY status, visibility ORDER BY events DESC;