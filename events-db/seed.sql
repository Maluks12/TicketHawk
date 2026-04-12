-- =============================================================================
-- seed.sql — Test data for EventsProject home page
--
-- HOW TO RUN:
--   psql "$DATABASE_URL" -f seed.sql
--
-- Or with explicit credentials:
--   psql postgresql://user:pass@localhost:5432/your_db -f seed.sql
--
-- WHAT IT CREATES:
--   • 6 categories  (Concerts, Sports, Gala, Tech, Food, Exclusive)
--   • 1 seed organizer user  (email: seed@events.dev  password: Seed1234!)
--   • 20 published events spread across all home page sections
--   • 1 featured event  (hero banner)
--   • High-view events  (trending + top 10)
--   • Johannesburg events  (near me)
--   • Weekend-dated events  (popular this weekend)
--   • Sports category events  (sports elite section)
--
-- SAFE TO RE-RUN: all inserts use ON CONFLICT DO NOTHING.
-- =============================================================================

BEGIN;

-- ─── 0. Extensions ──────────────────────────────────────────────────────────

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ─── 1. Categories ──────────────────────────────────────────────────────────
-- These IDs are fixed so the frontend constant SPORTS_CATEGORY_ID = 2 works.

INSERT INTO categories (id, name, slug) VALUES
  (1, 'Concerts',  'concerts'),
  (2, 'Sports',    'sports'),
  (3, 'Gala',      'gala'),
  (4, 'Tech',      'tech'),
  (5, 'Food',      'food'),
  (6, 'Exclusive', 'exclusive')
ON CONFLICT (id) DO NOTHING;

-- Keep the sequence in sync after manual ID inserts
SELECT setval(pg_get_serial_sequence('categories', 'id'), MAX(id)) FROM categories;

-- ─── 2. Seed Organizer User ─────────────────────────────────────────────────
-- Password: Seed1234!  (bcrypt cost 10)

INSERT INTO users (id, email, password, role) VALUES (
  '00000000-0000-0000-0000-000000000001',
  'seed@events.dev',
  crypt('Seed1234!', gen_salt('bf', 10)),
  'organizer'
)
ON CONFLICT (id) DO NOTHING;

-- ─── 3. Helper: slug generation ─────────────────────────────────────────────
-- Mirrors utils.GenerateSlug:  lower(title) → replace non-word with '-' → trim '-' → append first 8 chars of UUID

CREATE OR REPLACE FUNCTION make_slug(title TEXT, id UUID) RETURNS TEXT AS $$
  SELECT
    trim(BOTH '-' FROM regexp_replace(lower(title), '[^a-z0-9]+', '-', 'g'))
    || '-' ||
    substr(id::text, 1, 8);
$$ LANGUAGE SQL IMMUTABLE;

-- ─── 4. Events ──────────────────────────────────────────────────────────────
-- We pre-generate UUIDs so make_slug can reference them in the same statement.
-- cover_image_url / banner_image_url use picsum.photos — free, no auth required.
-- views drives the trending sort; is_featured = TRUE drives the hero banner.

-- Convenience: the organizer ID used throughout
DO $$ DECLARE org_id UUID := '00000000-0000-0000-0000-000000000001'; BEGIN

-- ── 4a. Featured Event (hero banner) ────────────────────────────────────────
INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, is_featured, featured_until,
  start_time, end_time, expires_at, published_at, views
) VALUES (
  'f1000000-0000-0000-0000-000000000001',
  make_slug('The World Finals 2026', 'f1000000-0000-0000-0000-000000000001'),
  'The World Finals 2026',
  'The most anticipated sporting event of the decade. Secure your premium hospitality passes for an unforgettable experience at the grandest stage in world football. VIP packages include paddock access, fine dining, and meet-and-greet opportunities.',
  'https://picsum.photos/seed/worldfinals/600/400',
  'https://picsum.photos/seed/worldfinalsbanner/1200/600',
  2, org_id,
  'Lusail Stadium', 'Lusail City', 'Lusail', 'Asia/Qatar',
  12500.00, 'ZAR', 80000, 61000,
  'published', TRUE, NOW() + INTERVAL '60 days',
  NOW() + INTERVAL '45 days', NOW() + INTERVAL '46 days', NOW() + INTERVAL '50 days',
  NOW() - INTERVAL '5 days', 98450
) ON CONFLICT (id) DO NOTHING;

-- ── 4b. Trending / High-View Events ─────────────────────────────────────────
-- These appear in TRENDING NOW and TOP 10 (sorted by views desc)

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'a1000000-0000-0000-0000-000000000001',
  make_slug('After Hours: Electronic Pulse', 'a1000000-0000-0000-0000-000000000001'),
  'After Hours: Electronic Pulse',
  'An immersive underground electronic music experience featuring headlining DJs from across Europe. Expect pounding bass, laser installations, and a crowd that knows how to move. Doors open at 22:00.',
  'https://picsum.photos/seed/electronicpulse/600/400',
  1, org_id, 'Fabric', '77A Charterhouse St', 'London', 'Europe/London',
  850.00, 'ZAR', 1500, 1380,
  'published', NOW() + INTERVAL '5 days', NOW() + INTERVAL '5 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '3 days', 87200 ),

( 'a1000000-0000-0000-0000-000000000002',
  make_slug('The Future of AI: Private Gala', 'a1000000-0000-0000-0000-000000000002'),
  'The Future of AI: Private Gala',
  'An invitation-only gathering of the world''s leading AI researchers, founders, and investors. Curated keynotes, closed-door roundtables, and a black-tie dinner at one of Paris''s most iconic venues. Space is strictly limited.',
  'https://picsum.photos/seed/aigala/600/400',
  4, org_id, 'Paris Convention Centre', '2 Place de la Porte Maillot', 'Paris', 'Europe/Paris',
  18000.00, 'ZAR', 200, 198,
  'published', NOW() + INTERVAL '12 days', NOW() + INTERVAL '12 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '15 days', NOW() - INTERVAL '7 days', 76300 ),

( 'a1000000-0000-0000-0000-000000000003',
  make_slug('Neon Pulse Festival', 'a1000000-0000-0000-0000-000000000003'),
  'Neon Pulse Festival',
  'Three stages. Forty artists. One unforgettable weekend. Neon Pulse is the Southern Hemisphere''s fastest-growing electronic music festival. Camping packages available. Must be 18+.',
  'https://picsum.photos/seed/neonpulse/600/400',
  1, org_id, 'Grand Central Arena', 'Grand Central Blvd', 'Johannesburg', 'Africa/Johannesburg',
  1200.00, 'ZAR', 15000, 12800,
  'published',
  -- Set to this coming Saturday for "weekend" filter
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '7 days',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '10 days', 65100 ),

( 'a1000000-0000-0000-0000-000000000004',
  make_slug('The Midnight Affair', 'a1000000-0000-0000-0000-000000000004'),
  'The Midnight Affair',
  'A masquerade ball like no other. Dress to impress as you mingle with Johannesburg''s elite in a transformed warehouse adorned with chandeliers and midnight drapes. Full bar, live jazz quartet, and a silent disco at midnight.',
  'https://picsum.photos/seed/midnightaffair/600/400',
  6, org_id, 'Metropolitan Club', '24 Central Ave', 'Johannesburg', 'Africa/Johannesburg',
  2500.00, 'ZAR', 400, 380,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '5 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '15 days', NOW() - INTERVAL '4 days', 59800 ),

( 'a1000000-0000-0000-0000-000000000005',
  make_slug('Championship Sprint Final', 'a1000000-0000-0000-0000-000000000005'),
  'Championship Sprint Final',
  'Watch the continent''s fastest athletes battle for glory on the track. Premium grandstand seating available with unobstructed views of the finish line. Hospitality suites and trackside packages also on offer.',
  'https://picsum.photos/seed/sprintfinal/600/400',
  2, org_id, 'National Stadium', 'Stadium Road', 'Johannesburg', 'Africa/Johannesburg',
  450.00, 'ZAR', 45000, 38000,
  'published', NOW() + INTERVAL '8 days', NOW() + INTERVAL '8 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '6 days', 53400 ),

( 'a1000000-0000-0000-0000-000000000006',
  make_slug('Black Tie Masquerade', 'a1000000-0000-0000-0000-000000000006'),
  'Black Tie Masquerade',
  'Johannesburg''s most prestigious annual gala returns for its 10th edition. An evening of fine dining, opera, and dancing in the Grand Ballroom. Black tie dress code strictly enforced. Tables of 10 available.',
  'https://picsum.photos/seed/blacktie/600/400',
  3, org_id, 'The Ritz', '151 Bree Street', 'Johannesburg', 'Africa/Johannesburg',
  3800.00, 'ZAR', 300, 295,
  'published',
  date_trunc('week', NOW()) + INTERVAL '6 days',
  date_trunc('week', NOW()) + INTERVAL '6 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '8 days', 48900 ),

( 'a1000000-0000-0000-0000-000000000007',
  make_slug('Global Founders Summit', 'a1000000-0000-0000-0000-000000000007'),
  'Global Founders Summit',
  'Two days of masterclasses, pitch competitions, and networking with over 500 startup founders and investors from 40 countries. Feature tracks include fintech, healthtech, climate, and deep tech. Startup exhibition floor open to the public on Day 2.',
  'https://picsum.photos/seed/founderssummit/600/400',
  4, org_id, 'Silicon Roundabout Hub', 'Old Street', 'London', 'Europe/London',
  2200.00, 'ZAR', 600, 521,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '21 days',
  NOW() + INTERVAL '25 days', NOW() - INTERVAL '2 days', 44700 ),

( 'a1000000-0000-0000-0000-000000000008',
  make_slug('Gourmet Garden Festival', 'a1000000-0000-0000-0000-000000000008'),
  'Gourmet Garden Festival',
  'A weekend-long open-air celebration of food, wine, and craft beverages. Over 80 vendors representing cuisines from 30 countries. Live cooking demonstrations every hour, plus a kids'' zone and a resident DJ from 14:00.',
  'https://picsum.photos/seed/gourmetgarden/600/400',
  5, org_id, 'City Park', 'Joubert Park', 'Johannesburg', 'Africa/Johannesburg',
  220.00, 'ZAR', 5000, 3800,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '6 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '15 days', NOW() - INTERVAL '5 days', 41200 ),

( 'a1000000-0000-0000-0000-000000000009',
  make_slug('Wimbledon Finals Night', 'a1000000-0000-0000-0000-000000000009'),
  'Wimbledon Finals Night',
  'Watch the Wimbledon gentlemen''s singles final live on the big screen in a premium outdoor setting with full hospitality service. Includes a three-course dinner, Pimms reception, and on-screen commentary. Dress code: garden party.',
  'https://picsum.photos/seed/wimbledon/600/400',
  2, org_id, 'All England Club', 'Church Rd', 'London', 'Europe/London',
  7500.00, 'ZAR', 200, 198,
  'published', NOW() + INTERVAL '30 days', NOW() + INTERVAL '30 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '35 days', NOW() - INTERVAL '9 days', 38600 ),

( 'a1000000-0000-0000-0000-000000000010',
  make_slug('The Indie Sessions', 'a1000000-0000-0000-0000-000000000010'),
  'The Indie Sessions',
  'Six rising indie acts. One intimate warehouse. This stripped-back showcase puts the spotlight on original South African songwriting and live musicianship. Standing room only. Bar opens at 17:00, acts from 19:00.',
  'https://picsum.photos/seed/indiesessions/600/400',
  1, org_id, 'Warehouse 4', '4 Main Reef Road', 'Johannesburg', 'Africa/Johannesburg',
  180.00, 'ZAR', 350, 298,
  'published', NOW() + INTERVAL '3 days', NOW() + INTERVAL '3 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '8 days', NOW() - INTERVAL '1 day', 35900 )

ON CONFLICT (id) DO NOTHING;

-- ── 4c. Near Me — additional Johannesburg events (city filter) ───────────────

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'b1000000-0000-0000-0000-000000000001',
  make_slug('Monaco Paddock Club Jhb Viewing', 'b1000000-0000-0000-0000-000000000001'),
  'Monaco Paddock Club Jhb Viewing',
  'Can''t make it to Monte Carlo? Experience the Monaco Grand Prix in style at our exclusive Johannesburg viewing event. Live feed on a 20m screen, full bar service, and F1 pundit commentary throughout the race weekend.',
  'https://picsum.photos/seed/monacojhb/600/400',
  6, org_id, 'Sandton Convention Centre', '161 Maude St', 'Johannesburg', 'Africa/Johannesburg',
  1800.00, 'ZAR', 300, 241,
  'published', NOW() + INTERVAL '14 days', NOW() + INTERVAL '14 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '3 days', 22100 ),

( 'b1000000-0000-0000-0000-000000000002',
  make_slug('The Clash in Vegas Jhb Watch Party', 'b1000000-0000-0000-0000-000000000002'),
  'The Clash in Vegas Jhb Watch Party',
  'The biggest heavyweight fight of the decade, watched live from Johannesburg. Premium ringside-style seating, commentary and analysis from local boxing legends. Late license until 06:00. 18+ only.',
  'https://picsum.photos/seed/boxingjhb/600/400',
  2, org_id, 'MGM-Style Arena JHB', '1 Empire Road', 'Johannesburg', 'Africa/Johannesburg',
  650.00, 'ZAR', 800, 612,
  'published', NOW() + INTERVAL '18 days', NOW() + INTERVAL '18 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '4 days', 19400 ),

( 'b1000000-0000-0000-0000-000000000003',
  make_slug('Jazz Under the Joburg Stars', 'b1000000-0000-0000-0000-000000000003'),
  'Jazz Under the Joburg Stars',
  'A sophisticated evening of live jazz in Johannesburg''s most beloved rooftop garden. Featuring a 7-piece big band and two guest vocalists, this is the perfect mid-week escape. Bring a blanket. Licensed bar and tapas menu available.',
  'https://picsum.photos/seed/jazzjoburg/600/400',
  1, org_id, 'Rooftop Arena', '55 Raleigh St', 'Johannesburg', 'Africa/Johannesburg',
  320.00, 'ZAR', 250, 190,
  'published', NOW() + INTERVAL '6 days', NOW() + INTERVAL '6 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '2 days', 17800 )

ON CONFLICT (id) DO NOTHING;

-- ── 4d. Popular this weekend — more weekend-dated events ────────────────────

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'c1000000-0000-0000-0000-000000000001',
  make_slug('Underground Beats Weekender', 'c1000000-0000-0000-0000-000000000001'),
  'Underground Beats Weekender',
  'Sixty continuous hours of underground house and techno across three interconnected basement rooms. International headliners, local favourites, and a curated art installation. No phone zone on the main floor.',
  'https://picsum.photos/seed/undergroundbeats/600/400',
  1, org_id, 'The Cellar', '12 Fox St', 'Johannesburg', 'Africa/Johannesburg',
  380.00, 'ZAR', 600, 540,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '7 days',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '6 days', 28700 ),

( 'c1000000-0000-0000-0000-000000000002',
  make_slug('Art Basel After Party Johannesburg', 'c1000000-0000-0000-0000-000000000002'),
  'Art Basel After Party Johannesburg',
  'Following the opening of Johannesburg Art Week, join collectors, artists, and gallerists for the official after-party. Works by 12 emerging local artists on display throughout the evening. Open bar for the first two hours.',
  'https://picsum.photos/seed/artbasel/600/400',
  6, org_id, 'Gallery One', '27 Jan Smuts Ave', 'Johannesburg', 'Africa/Johannesburg',
  1100.00, 'ZAR', 180, 160,
  'published',
  date_trunc('week', NOW()) + INTERVAL '6 days',
  date_trunc('week', NOW()) + INTERVAL '6 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '14 days', NOW() - INTERVAL '3 days', 23500 ),

( 'c1000000-0000-0000-0000-000000000003',
  make_slug('Comedy Night Live Johannesburg', 'c1000000-0000-0000-0000-000000000003'),
  'Comedy Night Live Johannesburg',
  'Five of South Africa''s hottest stand-up comedians in one room. The line-up is a surprise on the night — but we promise you''ll leave with sore cheeks. Two-drink minimum included in ticket price. Doors 19:00, show 20:00.',
  'https://picsum.photos/seed/comedynight/600/400',
  6, org_id, 'The Apollo JHB', '45 Rockey St', 'Johannesburg', 'Africa/Johannesburg',
  250.00, 'ZAR', 300, 267,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '5 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '1 day', 18900 )

ON CONFLICT (id) DO NOTHING;

-- ── 4e. Sports Elite section (category_id = 2) ──────────────────────────────
-- a1000000-...005 (Championship Sprint) and a1000000-...009 (Wimbledon) and
-- f1000000-...001 (World Finals) already cover sports. Add two more:

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'd1000000-0000-0000-0000-000000000001',
  make_slug('Pro League Finals', 'd1000000-0000-0000-0000-000000000001'),
  'Pro League Finals',
  'Courtside access is available for select members at the most electrifying basketball final of the South African season. Executive suites, pitch-side seating, and a post-match reception with players. Black card holders get priority access.',
  'https://picsum.photos/seed/proleague/600/400',
  'https://picsum.photos/seed/proleaguebanner/1200/600',
  2, org_id, 'Ellis Park Arena', 'Staib St, Doornfontein', 'Johannesburg', 'Africa/Johannesburg',
  3500.00, 'ZAR', 12000, 10800,
  'published', NOW() + INTERVAL '10 days', NOW() + INTERVAL '10 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '15 days', NOW() - INTERVAL '7 days', 31200 ),

( 'd1000000-0000-0000-0000-000000000002',
  make_slug('Gentlemen''s Singles Final View', 'd1000000-0000-0000-0000-000000000002'),
  'Gentlemen''s Singles Final View',
  'Watch history unfold on Centre Court with this premium Wimbledon hospitality package. Includes debenture seat, champagne breakfast, strawberries and cream, and an exclusive post-match dinner in the members'' enclosure.',
  'https://picsum.photos/seed/wimbledon2/600/400',
  NULL,
  2, org_id, 'All England Club', 'Church Rd, Wimbledon', 'London', 'Europe/London',
  28000.00, 'ZAR', 50, 48,
  'published', NOW() + INTERVAL '32 days', NOW() + INTERVAL '32 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '36 days', NOW() - INTERVAL '5 days', 27400 )

ON CONFLICT (id) DO NOTHING;

END $$;

-- ─── 5. Verify ──────────────────────────────────────────────────────────────

SELECT
  c.name                                          AS category,
  count(*)                                        AS event_count,
  sum(e.views)                                    AS total_views,
  max(CASE WHEN e.is_featured THEN 1 ELSE 0 END)  AS has_featured
FROM events e
JOIN categories c ON c.id = e.category_id
GROUP BY c.name
ORDER BY total_views DESC;

COMMIT;