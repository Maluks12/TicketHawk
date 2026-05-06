-- =============================================================================
-- seed_more_events.sql — Additional events for home page variety
--
-- HOW TO RUN IN DOCKER:
--   docker exec -i <container_name> psql -U <db_user> -d <db_name> < seed_more_events.sql
--
-- Find your container name:
--   docker ps
--
-- Example (adjust container name, user, dbname to match your docker-compose.yml):
--   docker exec -i events-db-postgres-1 psql -U postgres -d eventsdb < seed_more_events.sql
-- =============================================================================

BEGIN;

DO $$ DECLARE org_id UUID := '00000000-0000-0000-0000-000000000001'; BEGIN

-- =============================================================================
-- SECTION A — TRENDING / TOP 10
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'e1000000-0000-0000-0000-000000000001',
  make_slug('Afropunk Johannesburg 2026', 'e1000000-0000-0000-0000-000000000001'),
  'Afropunk Johannesburg 2026',
  'The groundbreaking festival celebrating Black punk, alternative, and art culture returns to Johannesburg for a third year.',
  'https://picsum.photos/seed/afropunk/600/400',
  1, org_id, 'Mary Fitzgerald Square', 'Newtown, JHB', 'Johannesburg', 'Africa/Johannesburg',
  580.00, 'ZAR', 8000, 7100,
  'published', NOW() + INTERVAL '9 days', NOW() + INTERVAL '10 days',
  NOW() + INTERVAL '14 days', NOW() - INTERVAL '12 days', 72400 ),

( 'e1000000-0000-0000-0000-000000000002',
  make_slug('Forbes Africa Power Gala 2026', 'e1000000-0000-0000-0000-000000000002'),
  'Forbes Africa Power Gala 2026',
  'Join Forbes Africa for its prestigious annual Power List dinner. Black tie required.',
  'https://picsum.photos/seed/forbesgala/600/400',
  3, org_id, 'Hyatt Regency Johannesburg', 'Corner of Biermann & Mutual Rd', 'Johannesburg', 'Africa/Johannesburg',
  8500.00, 'ZAR', 250, 230,
  'published', NOW() + INTERVAL '15 days', NOW() + INTERVAL '15 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '8 days', 68900 ),

( 'e1000000-0000-0000-0000-000000000003',
  make_slug('Africa Tech Week 2026', 'e1000000-0000-0000-0000-000000000003'),
  'Africa Tech Week 2026',
  'Three days of keynotes, workshops, and exhibitions across fintech, agritech, healthtech, and AI.',
  'https://picsum.photos/seed/africatech/600/400',
  4, org_id, 'Gallagher Convention Centre', 'Midrand', 'Johannesburg', 'Africa/Johannesburg',
  1800.00, 'ZAR', 5000, 4100,
  'published', NOW() + INTERVAL '22 days', NOW() + INTERVAL '24 days',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '14 days', 61700 ),

( 'e1000000-0000-0000-0000-000000000004',
  make_slug('Taste of Johannesburg 2026', 'e1000000-0000-0000-0000-000000000004'),
  'Taste of Johannesburg 2026',
  'South Africa''s premier food and wine festival with masterclasses, tastings, and a street food village.',
  'https://picsum.photos/seed/tastejhb/600/400',
  5, org_id, 'Montecasino', 'William Nicol Drive, Fourways', 'Johannesburg', 'Africa/Johannesburg',
  450.00, 'ZAR', 4000, 3600,
  'published', NOW() + INTERVAL '11 days', NOW() + INTERVAL '12 days',
  NOW() + INTERVAL '16 days', NOW() - INTERVAL '10 days', 57300 ),

( 'e1000000-0000-0000-0000-000000000005',
  make_slug('Lions vs Sharks Super Rugby Final', 'e1000000-0000-0000-0000-000000000005'),
  'Lions vs Sharks Super Rugby Final',
  'South Africa''s two biggest Super Rugby franchises clash in the season finale at Ellis Park.',
  'https://picsum.photos/seed/rugbyfinal/600/400',
  2, org_id, 'Ellis Park Stadium', 'Staib St, Doornfontein', 'Johannesburg', 'Africa/Johannesburg',
  950.00, 'ZAR', 62000, 54000,
  'published', NOW() + INTERVAL '7 days', NOW() + INTERVAL '7 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '5 days', 52800 ),

( 'e1000000-0000-0000-0000-000000000006',
  make_slug('Burna Boy Live in Johannesburg', 'e1000000-0000-0000-0000-000000000006'),
  'Burna Boy Live in Johannesburg',
  'Grammy Award-winning Afrobeats superstar Burna Boy brings his world-tour to South Africa for one night only.',
  'https://picsum.photos/seed/burnaboy/600/400',
  1, org_id, 'FNB Stadium', 'Soccer City Ave, Nasrec', 'Johannesburg', 'Africa/Johannesburg',
  1350.00, 'ZAR', 90000, 82000,
  'published', NOW() + INTERVAL '17 days', NOW() + INTERVAL '17 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '7 days', 49600 ),

( 'e1000000-0000-0000-0000-000000000007',
  make_slug('Soho House Johannesburg Launch', 'e1000000-0000-0000-0000-000000000007'),
  'Soho House Johannesburg Launch',
  'The world-famous members'' club opens its first African outpost in Johannesburg.',
  'https://picsum.photos/seed/sohohouse/600/400',
  6, org_id, 'Soho House JHB', 'Sandton', 'Johannesburg', 'Africa/Johannesburg',
  22000.00, 'ZAR', 120, 118,
  'published', NOW() + INTERVAL '25 days', NOW() + INTERVAL '25 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '30 days', NOW() - INTERVAL '3 days', 46100 ),

( 'e1000000-0000-0000-0000-000000000008',
  make_slug('Night Market: Night City Edition', 'e1000000-0000-0000-0000-000000000008'),
  'Night Market: Night City Edition',
  'Over 60 street food vendors, craft cocktail bars, live music across two stages, and a street art zone.',
  'https://picsum.photos/seed/nightmarket/600/400',
  5, org_id, 'The Sheds', '80 Durham St, Auckland Park', 'Johannesburg', 'Africa/Johannesburg',
  120.00, 'ZAR', 3000, 2100,
  'published', NOW() + INTERVAL '4 days', NOW() + INTERVAL '4 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '8 days', NOW() - INTERVAL '2 days', 43800 ),

( 'e1000000-0000-0000-0000-000000000009',
  make_slug('Miss South Africa 2026 Finale', 'e1000000-0000-0000-0000-000000000009'),
  'Miss South Africa 2026 Finale',
  'Witness the crowning of Miss South Africa 2026 live at Sun City.',
  'https://picsum.photos/seed/missa/600/400',
  3, org_id, 'Sun City Superbowl', 'Sun City Resort', 'Sun City', 'Africa/Johannesburg',
  2200.00, 'ZAR', 6000, 5400,
  'published', NOW() + INTERVAL '19 days', NOW() + INTERVAL '19 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '9 days', 40200 ),

( 'e1000000-0000-0000-0000-000000000010',
  make_slug('House on the Hill: Volume 12', 'e1000000-0000-0000-0000-000000000010'),
  'House on the Hill: Volume 12',
  'The legendary Johannesburg day-to-night house event. 100% South African lineups. 12:00 to 04:00.',
  'https://picsum.photos/seed/houseonhill/600/400',
  1, org_id, 'Private Estate Midrand', 'Midrand', 'Johannesburg', 'Africa/Johannesburg',
  680.00, 'ZAR', 2500, 2200,
  'published', NOW() + INTERVAL '13 days', NOW() + INTERVAL '14 days',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '6 days', 37500 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- SECTION B — HAPPENING SOON
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'f2000000-0000-0000-0000-000000000001',
  make_slug('Sunset Sessions: Soulful Vibes', 'f2000000-0000-0000-0000-000000000001'),
  'Sunset Sessions: Soulful Vibes',
  'An intimate rooftop experience featuring neo-soul and R&B artists from across Johannesburg.',
  'https://picsum.photos/seed/sunsetvibes/600/400',
  1, org_id, 'Marble Restaurant Rooftop', '100 Bishops Ave, Rosebank', 'Johannesburg', 'Africa/Johannesburg',
  350.00, 'ZAR', 150, 112,
  'published', NOW() + INTERVAL '2 days', NOW() + INTERVAL '2 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '5 days', NOW() - INTERVAL '1 day', 8900 ),

( 'f2000000-0000-0000-0000-000000000002',
  make_slug('Morning Rave: Sunrise Edition', 'f2000000-0000-0000-0000-000000000002'),
  'Morning Rave: Sunrise Edition',
  'Dance the sunrise away at Joburg''s most unique early-morning rave. 05:00–09:00. No alcohol.',
  'https://picsum.photos/seed/morningrave/600/400',
  1, org_id, 'The Lyric Theatre', 'Gold Reef City', 'Johannesburg', 'Africa/Johannesburg',
  220.00, 'ZAR', 800, 630,
  'published', NOW() + INTERVAL '1 day', NOW() + INTERVAL '1 day' + INTERVAL '4 hours',
  NOW() + INTERVAL '3 days', NOW() - INTERVAL '3 days', 12400 ),

( 'f2000000-0000-0000-0000-000000000003',
  make_slug('Startup Pitch Night Johannesburg', 'f2000000-0000-0000-0000-000000000003'),
  'Startup Pitch Night Johannesburg',
  'Ten startups. Ten minutes each. One R500 000 prize. Networking drinks from 18:30.',
  'https://picsum.photos/seed/pitchnight/600/400',
  4, org_id, 'The Workshop', '1 Keyes Ave, Rosebank', 'Johannesburg', 'Africa/Johannesburg',
  0.00, 'ZAR', 300, 245,
  'published', NOW() + INTERVAL '3 days', NOW() + INTERVAL '3 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '6 days', NOW() - INTERVAL '2 days', 6700 ),

( 'f2000000-0000-0000-0000-000000000004',
  make_slug('Soweto Wine & Lifestyle Festival', 'f2000000-0000-0000-0000-000000000004'),
  'Soweto Wine & Lifestyle Festival',
  'South Africa''s finest wineries alongside a curated food market, jazz performances, and cocktail pop-ups.',
  'https://picsum.photos/seed/sowetowine/600/400',
  5, org_id, 'Ubuntu Kraal', 'Soweto', 'Soweto', 'Africa/Johannesburg',
  280.00, 'ZAR', 2500, 1800,
  'published', NOW() + INTERVAL '5 days', NOW() + INTERVAL '5 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '9 days', NOW() - INTERVAL '4 days', 15300 ),

( 'f2000000-0000-0000-0000-000000000005',
  make_slug('Polo in the Park Joburg', 'f2000000-0000-0000-0000-000000000005'),
  'Polo in the Park Joburg',
  'A glamorous afternoon of polo at Inanda Club. Dress code: garden party. Hat competition at 15:00.',
  'https://picsum.photos/seed/polo/600/400',
  2, org_id, 'Inanda Club', 'Forrest Rd, Inanda', 'Johannesburg', 'Africa/Johannesburg',
  1600.00, 'ZAR', 500, 320,
  'published', NOW() + INTERVAL '6 days', NOW() + INTERVAL '6 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '3 days', 11800 ),

( 'f2000000-0000-0000-0000-000000000006',
  make_slug('Joburg Art Fair Preview Night', 'f2000000-0000-0000-0000-000000000006'),
  'Joburg Art Fair Preview Night',
  'Exclusive preview for the 15th Joburg Art Fair. First access to works by 200 galleries from 45 countries.',
  'https://picsum.photos/seed/artfair/600/400',
  6, org_id, 'Sandton Convention Centre', '161 Maude St', 'Sandton', 'Africa/Johannesburg',
  4500.00, 'ZAR', 400, 385,
  'published', NOW() + INTERVAL '4 days', NOW() + INTERVAL '4 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '8 days', NOW() - INTERVAL '5 days', 21600 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- SECTION C — THIS WEEKEND
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'a1000000-0000-0000-0000-000000000001',
  make_slug('Braamfontein Food Truck Fest', 'a1000000-0000-0000-0000-000000000001'),
  'Braamfontein Food Truck Fest',
  'Twenty of Joburg''s best food trucks with craft beer, live music, and kids'' activities. Free entry.',
  'https://picsum.photos/seed/foodtrucks/600/400',
  5, org_id, 'Neighbourgoods Market', '73 Juta St, Braamfontein', 'Braamfontein', 'Africa/Johannesburg',
  0.00, 'ZAR', 4000, 0,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '5 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '6 days', 19200 ),

( 'a1000000-0000-0000-0000-000000000002',
  make_slug('eSports Arena: Valorant Joburg Open', 'a1000000-0000-0000-0000-000000000002'),
  'eSports Arena: Valorant Joburg Open',
  'South Africa''s biggest Valorant tournament. 32 teams, R250 000 prize pool. Live on Twitch.',
  'https://picsum.photos/seed/esports/600/400',
  4, org_id, 'Ticketpro Dome', 'Northgate', 'Johannesburg', 'Africa/Johannesburg',
  180.00, 'ZAR', 3000, 2600,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '6 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '7 days', 24500 ),

( 'a1000000-0000-0000-0000-000000000003',
  make_slug('Kefilwe Mabote Luxury Retreat Weekend', 'a1000000-0000-0000-0000-000000000003'),
  'Kefilwe Mabote Luxury Retreat Weekend',
  'A curated weekend wellness and luxury retreat. Yoga, sound healing, panel discussions, and luxury brand pop-ups.',
  'https://picsum.photos/seed/luxuryretreat/600/400',
  6, org_id, 'The Peech Hotel', '61 North St, Melrose', 'Johannesburg', 'Africa/Johannesburg',
  3200.00, 'ZAR', 80, 72,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '6 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '11 days', NOW() - INTERVAL '4 days', 16800 ),

( 'a1000000-0000-0000-0000-000000000004',
  make_slug('Tri-Nations Athletics Meet JHB', 'a1000000-0000-0000-0000-000000000004'),
  'Tri-Nations Athletics Meet JHB',
  'South Africa, Kenya, and Ethiopia clash in this elite athletics invitational at Dobsonville Stadium.',
  'https://picsum.photos/seed/athletics/600/400',
  2, org_id, 'Dobsonville Stadium', 'Dobsonville, Soweto', 'Soweto', 'Africa/Johannesburg',
  320.00, 'ZAR', 8000, 5900,
  'published',
  date_trunc('week', NOW()) + INTERVAL '6 days',
  date_trunc('week', NOW()) + INTERVAL '6 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '5 days', 22100 ),

( 'a1000000-0000-0000-0000-000000000005',
  make_slug('Ballroom Blitz: Salsa & Bachata Night', 'a1000000-0000-0000-0000-000000000005'),
  'Ballroom Blitz: Salsa & Bachata Night',
  'Joburg''s hottest Latin dance party. Free beginner lesson at 19:00. Four DJs across two floors.',
  'https://picsum.photos/seed/salsa/600/400',
  1, org_id, 'Latin Quarter JHB', '4 Fricker Rd, Illovo', 'Johannesburg', 'Africa/Johannesburg',
  150.00, 'ZAR', 400, 280,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '5 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '9 days', NOW() - INTERVAL '2 days', 9800 ),

( 'a1000000-0000-0000-0000-000000000006',
  make_slug('Craft Beer & Biltong Festival', 'a1000000-0000-0000-0000-000000000006'),
  'Craft Beer & Biltong Festival',
  'Over 40 SA craft breweries with biltong producers, live rugby, braai competition. Dog-friendly.',
  'https://picsum.photos/seed/craftbeer/600/400',
  5, org_id, 'Marks Park Sports Club', 'Brixton, JHB', 'Johannesburg', 'Africa/Johannesburg',
  200.00, 'ZAR', 3500, 2800,
  'published',
  date_trunc('week', NOW()) + INTERVAL '6 days',
  date_trunc('week', NOW()) + INTERVAL '6 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '11 days', NOW() - INTERVAL '3 days', 27300 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- SECTION D — RECENTLY ADDED
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'b1000000-0000-0000-0000-000000000001',
  make_slug('Open Air Cinema: Lion King JHB', 'b1000000-0000-0000-0000-000000000001'),
  'Open Air Cinema: Lion King JHB',
  'Watch the classic Lion King under the Johannesburg stars. Gates open 18:00, movie starts 20:00.',
  'https://picsum.photos/seed/opencinema/600/400',
  6, org_id, 'Zoo Lake', 'Herman Eckstein Park', 'Johannesburg', 'Africa/Johannesburg',
  130.00, 'ZAR', 1000, 120,
  'published', NOW() + INTERVAL '16 days', NOW() + INTERVAL '16 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '6 hours', 880 ),

( 'b1000000-0000-0000-0000-000000000002',
  make_slug('Women in Tech Summit Cape Town', 'b1000000-0000-0000-0000-000000000002'),
  'Women in Tech Summit Cape Town',
  'A day of inspiration, skill-building, and networking for women in the technology sector.',
  'https://picsum.photos/seed/womenintech/600/400',
  4, org_id, 'CTICC', 'Convention Square', 'Cape Town', 'Africa/Johannesburg',
  650.00, 'ZAR', 500, 85,
  'published', NOW() + INTERVAL '28 days', NOW() + INTERVAL '28 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '4 hours', 540 ),

( 'b1000000-0000-0000-0000-000000000003',
  make_slug('Dark Room: Sensory Dining Experience', 'b1000000-0000-0000-0000-000000000003'),
  'Dark Room: Sensory Dining Experience',
  'A completely dark, six-course fine dining experience. Dietary requirements accommodated.',
  'https://picsum.photos/seed/darkroom/600/400',
  5, org_id, 'The Test Kitchen Pop-Up', 'Melville', 'Johannesburg', 'Africa/Johannesburg',
  1850.00, 'ZAR', 40, 12,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '20 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '2 hours', 320 ),

( 'b1000000-0000-0000-0000-000000000004',
  make_slug('Flohmarkt Johannesburg Pop-Up Market', 'b1000000-0000-0000-0000-000000000004'),
  'Flohmarkt Johannesburg Pop-Up Market',
  'Vintage clothing, rare records, handmade ceramics. Over 80 stalls in Maboneng.',
  'https://picsum.photos/seed/flohmarkt/600/400',
  6, org_id, 'Maboneng Precinct', 'Fox St', 'Johannesburg', 'Africa/Johannesburg',
  50.00, 'ZAR', 2000, 0,
  'published', NOW() + INTERVAL '18 days', NOW() + INTERVAL '18 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '1 hour', 210 ),

( 'b1000000-0000-0000-0000-000000000005',
  make_slug('Karate World Championships JHB Qualifier', 'b1000000-0000-0000-0000-000000000005'),
  'Karate World Championships JHB Qualifier',
  'South Africa''s national karate qualifier for the World Championships. Kata and kumite disciplines.',
  'https://picsum.photos/seed/karate/600/400',
  2, org_id, 'Wits Sports Complex', 'Yale Rd, Braamfontein', 'Johannesburg', 'Africa/Johannesburg',
  80.00, 'ZAR', 600, 45,
  'published', NOW() + INTERVAL '14 days', NOW() + INTERVAL '15 days',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '30 minutes', 90 ),

( 'b1000000-0000-0000-0000-000000000006',
  make_slug('Tristan Tait Piano Recital', 'b1000000-0000-0000-0000-000000000006'),
  'Tristan Tait Piano Recital',
  'Award-winning SA pianist performs Chopin, Ravel, and Prokofiev. Wine included.',
  'https://picsum.photos/seed/pianrecital/600/400',
  1, org_id, 'The Wits Theatre', 'University Estate', 'Johannesburg', 'Africa/Johannesburg',
  380.00, 'ZAR', 95, 28,
  'published', NOW() + INTERVAL '23 days', NOW() + INTERVAL '23 days' + INTERVAL '2 hours',
  NOW() + INTERVAL '27 days', NOW() - INTERVAL '15 minutes', 150 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- SECTION E — CAPE TOWN & DURBAN
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'c1000000-0000-0000-0000-000000000001',
  make_slug('Cape Town Jazz Festival 2026', 'c1000000-0000-0000-0000-000000000001'),
  'Cape Town Jazz Festival 2026',
  'The largest jazz event on the African continent. 40+ artists across six stages at the CTICC.',
  'https://picsum.photos/seed/ctjazz/600/400',
  1, org_id, 'CTICC', 'Convention Square', 'Cape Town', 'Africa/Johannesburg',
  1290.00, 'ZAR', 35000, 28000,
  'published', NOW() + INTERVAL '26 days', NOW() + INTERVAL '27 days',
  NOW() + INTERVAL '31 days', NOW() - INTERVAL '20 days', 88300 ),

( 'c1000000-0000-0000-0000-000000000002',
  make_slug('Surfing South Africa: Durban Open', 'c1000000-0000-0000-0000-000000000002'),
  'Surfing South Africa: Durban Open',
  'World Surf League-ranked athletes compete at North Beach. Free viewing from the promenade.',
  'https://picsum.photos/seed/surfopen/600/400',
  2, org_id, 'North Beach Durban', 'Marine Parade', 'Durban', 'Africa/Johannesburg',
  0.00, 'ZAR', 10000, 0,
  'published', NOW() + INTERVAL '31 days', NOW() + INTERVAL '33 days',
  NOW() + INTERVAL '36 days', NOW() - INTERVAL '15 days', 34100 ),

( 'c1000000-0000-0000-0000-000000000003',
  make_slug('Franschhoek Cap Classique Festival', 'c1000000-0000-0000-0000-000000000003'),
  'Franschhoek Cap Classique Festival',
  'Tastings from 30+ estates, guided cellar tours, and a five-course MCC-paired dinner.',
  'https://picsum.photos/seed/franschhoek/600/400',
  5, org_id, 'Huguenot Square', 'Franschhoek', 'Franschhoek', 'Africa/Johannesburg',
  880.00, 'ZAR', 2000, 1650,
  'published', NOW() + INTERVAL '24 days', NOW() + INTERVAL '25 days',
  NOW() + INTERVAL '29 days', NOW() - INTERVAL '11 days', 29800 ),

( 'c1000000-0000-0000-0000-000000000004',
  make_slug('Design Indaba 2026 Cape Town', 'c1000000-0000-0000-0000-000000000004'),
  'Design Indaba 2026 Cape Town',
  'Africa''s leading design conference. 30 international speakers and a parallel public expo.',
  'https://picsum.photos/seed/designindaba/600/400',
  4, org_id, 'Artscape Theatre', 'DF Malan St', 'Cape Town', 'Africa/Johannesburg',
  3500.00, 'ZAR', 1200, 980,
  'published', NOW() + INTERVAL '35 days', NOW() + INTERVAL '37 days',
  NOW() + INTERVAL '41 days', NOW() - INTERVAL '18 days', 41900 ),

( 'c1000000-0000-0000-0000-000000000005',
  make_slug('Springboks vs Ireland Test Match', 'c1000000-0000-0000-0000-000000000005'),
  'Springboks vs Ireland Test Match',
  'The world-champion Springboks face Ireland in a blockbuster test match at DHL Newlands.',
  'https://picsum.photos/seed/springboks/600/400',
  2, org_id, 'DHL Newlands Stadium', 'Boundary Rd, Newlands', 'Cape Town', 'Africa/Johannesburg',
  2800.00, 'ZAR', 51900, 48000,
  'published', NOW() + INTERVAL '29 days', NOW() + INTERVAL '29 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '33 days', NOW() - INTERVAL '22 days', 76500 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- SECTION F — FREE EVENTS
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'd1000000-0000-0000-0000-000000000001',
  make_slug('Free Yoga in the Park JHB', 'd1000000-0000-0000-0000-000000000001'),
  'Free Yoga in the Park JHB',
  'Community yoga session every Saturday morning in Emmarentia Park. All levels welcome.',
  'https://picsum.photos/seed/yogapark/600/400',
  6, org_id, 'Emmarentia Dam', 'Emmarentia', 'Johannesburg', 'Africa/Johannesburg',
  0.00, 'ZAR', 100, 0,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days' + INTERVAL '7 hours',
  date_trunc('week', NOW()) + INTERVAL '5 days' + INTERVAL '9 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '5 days', 3200 ),

( 'd1000000-0000-0000-0000-000000000002',
  make_slug('Open Mic Night Maboneng', 'd1000000-0000-0000-0000-000000000002'),
  'Open Mic Night Maboneng',
  'Every Thursday — poetry, music, stand-up, spoken word. Sign-up at door from 19:30. Free entry.',
  'https://picsum.photos/seed/openmic/600/400',
  1, org_id, 'Living Room Bar', 'Fox St, Maboneng', 'Johannesburg', 'Africa/Johannesburg',
  0.00, 'ZAR', 120, 0,
  'published', NOW() + INTERVAL '3 days', NOW() + INTERVAL '3 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '6 days', NOW() - INTERVAL '1 day', 2800 ),

( 'd1000000-0000-0000-0000-000000000003',
  make_slug('Soweto Marathon 2026', 'd1000000-0000-0000-0000-000000000003'),
  'Soweto Marathon 2026',
  'One of SA''s most celebrated road races through the vibrant streets of Soweto. 10km, 21km, 42.2km.',
  'https://picsum.photos/seed/sowetom/600/400',
  2, org_id, 'Orlando Stadium', 'Soweto', 'Soweto', 'Africa/Johannesburg',
  280.00, 'ZAR', 20000, 16000,
  'published', NOW() + INTERVAL '38 days', NOW() + INTERVAL '38 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '42 days', NOW() - INTERVAL '25 days', 31200 )

ON CONFLICT (id) DO NOTHING;

END $$;

-- Verify
SELECT c.name AS category, COUNT(*) AS events, SUM(e.views) AS total_views
FROM events e
JOIN categories c ON c.id = e.category_id
WHERE e.status = 'published'
GROUP BY c.name ORDER BY total_views DESC;

COMMIT;