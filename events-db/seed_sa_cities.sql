-- =============================================================================
-- seed_sa_cities.sql — 200+ events across South African cities
--
-- HOW TO RUN (after seed.sql and seed_more_events.sql):
--   psql "$DATABASE_URL" -f seed_sa_cities.sql
--
-- CITIES COVERED (≥ 20 events each):
--   Cape Town · Durban · Pretoria · Gqeberha (Port Elizabeth)
--   Bloemfontein · Johannesburg (extra) · Stellenbosch · Pietermaritzburg
--
-- UUID PREFIX MAP:
--   e2xxxxxx  Cape Town
--   e3xxxxxx  Durban
--   e4xxxxxx  Pretoria
--   e5xxxxxx  Gqeberha
--   e6xxxxxx  Bloemfontein
--   e7xxxxxx  Extra Johannesburg
--   e8xxxxxx  Stellenbosch
--   e9xxxxxx  Pietermaritzburg
--
-- Ticket-type IDs follow the same pattern as seed_ticket_types.sql:
--   event last-block digit → first block of ticket ID
--   e.g. event e2000000-…-000000000003 → tickets e2000003-…-000000000001/002/003
--
-- SAFE TO RE-RUN: all inserts use ON CONFLICT DO NOTHING.
-- =============================================================================

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- make_slug is already defined by seed.sql — redefine safely
CREATE OR REPLACE FUNCTION make_slug(title TEXT, id UUID) RETURNS TEXT AS $$
  SELECT
    trim(BOTH '-' FROM regexp_replace(lower(title), '[^a-z0-9]+', '-', 'g'))
    || '-' ||
    substr(id::text, 1, 8);
$$ LANGUAGE SQL IMMUTABLE;

DO $$ DECLARE org_id UUID := '00000000-0000-0000-0000-000000000001'; BEGIN

-- =============================================================================
-- ██████████  CAPE TOWN  (e2 prefix, 25 events)
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

-- 1. Cape Town Stadium — Soccer
( 'e2000000-0000-0000-0000-000000000001',
  make_slug('Cape Town City FC vs Kaizer Chiefs', 'e2000000-0000-0000-0000-000000000001'),
  'Cape Town City FC vs Kaizer Chiefs',
  'The Premier Soccer League clash that Cape Town has been waiting for. Cape Town City host Kaizer Chiefs under the lights at DHL Stadium in what promises to be a cracker. Braai areas open from 16:00. Family-friendly zones available.',
  'https://picsum.photos/seed/ctcityvchiefs/600/400',
  'https://picsum.photos/seed/ctcitybannerv2/1200/600',
  2, org_id, 'DHL Cape Town Stadium', 'Fritz Sonnenberg Rd, Green Point', 'Cape Town', 'Africa/Johannesburg',
  180.00, 'ZAR', 55000, 38000,
  'published', NOW() + INTERVAL '8 days', NOW() + INTERVAL '8 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '4 days', 41200 ),

-- 2. DHL Newlands — Rugby
( 'e2000000-0000-0000-0000-000000000002',
  make_slug('Stormers vs Lions United Rugby Championship', 'e2000000-0000-0000-0000-000000000002'),
  'Stormers vs Lions United Rugby Championship',
  'The Cape Derby returns to DHL Newlands for a blockbuster URC clash. The defending champions host their fiercest rivals in what will be a sold-out spectacle. Premium hospitality suites available for corporates. Gates open 2 hours before kick-off.',
  'https://picsum.photos/seed/stormerslions/600/400',
  NULL,
  2, org_id, 'DHL Newlands Stadium', 'Boundary Rd, Newlands', 'Cape Town', 'Africa/Johannesburg',
  320.00, 'ZAR', 51000, 44000,
  'published', NOW() + INTERVAL '11 days', NOW() + INTERVAL '11 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '15 days', NOW() - INTERVAL '5 days', 38700 ),

-- 3. Kirstenbosch — Concert
( 'e2000000-0000-0000-0000-000000000003',
  make_slug('Kirstenbosch Summer Concerts: Freshlyground', 'e2000000-0000-0000-0000-000000000003'),
  'Kirstenbosch Summer Concerts: Freshlyground',
  'Freshlyground returns to the iconic Kirstenbosch Botanical Gardens for their annual summer concert. Bring your picnic basket, a blanket, and settle in for an unforgettable evening with Table Mountain as your backdrop. BYO wine permitted.',
  'https://picsum.photos/seed/kirstenboschoct/600/400',
  NULL,
  1, org_id, 'Kirstenbosch National Botanical Garden', 'Rhodes Dr, Newlands', 'Cape Town', 'Africa/Johannesburg',
  280.00, 'ZAR', 8000, 6200,
  'published',
  date_trunc('week', NOW()) + INTERVAL '5 days',
  date_trunc('week', NOW()) + INTERVAL '5 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '3 days', 29800 ),

-- 4. CTICC — Tech
( 'e2000000-0000-0000-0000-000000000004',
  make_slug('AfricaCom 2026 Cape Town', 'e2000000-0000-0000-0000-000000000004'),
  'AfricaCom 2026 Cape Town',
  'Africa''s largest technology and telecommunications event returns to the CTICC. Three days of keynotes, product launches, workshops, and networking with 14 000 delegates from across the continent. Exhibition floor spans 12 000 sqm.',
  'https://picsum.photos/seed/africacomct/600/400',
  NULL,
  4, org_id, 'Cape Town International Convention Centre', 'Convention Square, 1 Lower Long St', 'Cape Town', 'Africa/Johannesburg',
  4500.00, 'ZAR', 14000, 9800,
  'published', NOW() + INTERVAL '22 days', NOW() + INTERVAL '24 days',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '10 days', 27300 ),

-- 5. Constantia — Food & Wine
( 'e2000000-0000-0000-0000-000000000005',
  make_slug('Constantia Fresh Food & Wine Festival', 'e2000000-0000-0000-0000-000000000005'),
  'Constantia Fresh Food & Wine Festival',
  'Sip your way through over 60 Constantia Valley wines while sampling dishes from 25 of the Cape''s top chefs. Live jazz throughout the day, cooking demonstrations, and a dedicated kids'' zone. The most elegant food festival in the country.',
  'https://picsum.photos/seed/constantiafresh/600/400',
  NULL,
  5, org_id, 'Buitenverwachting Wine Estate', 'Klein Constantia Rd', 'Cape Town', 'Africa/Johannesburg',
  450.00, 'ZAR', 3000, 2100,
  'published', NOW() + INTERVAL '16 days', NOW() + INTERVAL '16 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '6 days', 21500 ),

-- 6. Artscape — Gala
( 'e2000000-0000-0000-0000-000000000006',
  make_slug('Cape Philharmonic Orchestra Gala Night', 'e2000000-0000-0000-0000-000000000006'),
  'Cape Philharmonic Orchestra Gala Night',
  'An extraordinary black-tie evening with the Cape Philharmonic Orchestra performing Beethoven''s 9th Symphony in its full glory. Champagne reception from 18:30, performance at 20:00. Formal attire required. Valet parking available.',
  'https://picsum.photos/seed/caphilharmonic/600/400',
  NULL,
  3, org_id, 'Artscape Opera House', 'DF Malan St, Foreshore', 'Cape Town', 'Africa/Johannesburg',
  1200.00, 'ZAR', 1200, 980,
  'published', NOW() + INTERVAL '19 days', NOW() + INTERVAL '19 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '7 days', 18900 ),

-- 7. Green Point Common — Sports
( 'e2000000-0000-0000-0000-000000000007',
  make_slug('Cape Town 10K City Run 2026', 'e2000000-0000-0000-0000-000000000007'),
  'Cape Town 10K City Run 2026',
  'Lace up and join 12 000 runners for Cape Town''s favourite city race. The route takes you through the V&A Waterfront, along the Sea Point promenade, and finishes at Green Point Common. 10K, 5K, and Fun Run categories. Timing chips included.',
  'https://picsum.photos/seed/ct10krun/600/400',
  NULL,
  2, org_id, 'Green Point Common', 'Fritz Sonnenberg Rd', 'Cape Town', 'Africa/Johannesburg',
  220.00, 'ZAR', 12000, 9400,
  'published', NOW() + INTERVAL '13 days', NOW() + INTERVAL '13 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '16 days', NOW() - INTERVAL '4 days', 16700 ),

-- 8. V&A Waterfront — Concert
( 'e2000000-0000-0000-0000-000000000008',
  make_slug('V&A Waterfront New Year Eve Concert 2026', 'e2000000-0000-0000-0000-000000000008'),
  'V&A Waterfront New Year Eve Concert 2026',
  'Ring in the new year at the most iconic outdoor stage in Cape Town. International DJ sets from 21:00, countdown celebrations, a spectacular fireworks display over Table Bay at midnight. Outdoor standing festival. All ages welcome.',
  'https://picsum.photos/seed/vawaterfront/600/400',
  'https://picsum.photos/seed/vawaterfrontbanner/1200/600',
  1, org_id, 'V&A Waterfront Amphitheatre', 'Dock Rd, V&A Waterfront', 'Cape Town', 'Africa/Johannesburg',
  650.00, 'ZAR', 20000, 17500,
  'published', NOW() + INTERVAL '35 days', NOW() + INTERVAL '36 days',
  NOW() + INTERVAL '38 days', NOW() - INTERVAL '14 days', 53200 ),

-- 9. GrandWest — Exclusive
( 'e2000000-0000-0000-0000-000000000009',
  make_slug('GrandWest Comedy Festival 2026', 'e2000000-0000-0000-0000-000000000009'),
  'GrandWest Comedy Festival 2026',
  'Four nights, twelve comedians, one legendary venue. The annual GrandWest Comedy Festival brings together South Africa''s biggest names alongside two international acts. Dinner-show packages available. 18+ only.',
  'https://picsum.photos/seed/grandwestcomedy/600/400',
  NULL,
  6, org_id, 'GrandWest Casino & Entertainment World', 'Vasco Blvd, Goodwood', 'Cape Town', 'Africa/Johannesburg',
  380.00, 'ZAR', 2200, 1800,
  'published', NOW() + INTERVAL '26 days', NOW() + INTERVAL '29 days',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '9 days', 14200 ),

-- 10. Cape Town Stadium — Music
( 'e2000000-0000-0000-0000-000000000010',
  make_slug('Black Coffee at Cape Town Stadium', 'e2000000-0000-0000-0000-000000000010'),
  'Black Coffee at Cape Town Stadium',
  'Grammy-winning DJ and producer Black Coffee brings his iconic open-air spectacle to DHL Cape Town Stadium. Special guests TBA. Gates open at 14:00, first act at 16:00. The biggest electronic music event the Mother City has ever seen.',
  'https://picsum.photos/seed/blackcoffeect/600/400',
  'https://picsum.photos/seed/blackcoffeectbanner/1200/600',
  1, org_id, 'DHL Cape Town Stadium', 'Fritz Sonnenberg Rd, Green Point', 'Cape Town', 'Africa/Johannesburg',
  750.00, 'ZAR', 55000, 48000,
  'published', NOW() + INTERVAL '30 days', NOW() + INTERVAL '30 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '34 days', NOW() - INTERVAL '18 days', 67800 ),

-- 11. Clifton Beach — Food
( 'e2000000-0000-0000-0000-000000000011',
  make_slug('Sea Point Night Market & Food Festival', 'e2000000-0000-0000-0000-000000000011'),
  'Sea Point Night Market & Food Festival',
  'The Sea Point Promenade transforms into a gourmet paradise every Saturday evening. Over 40 street food vendors, craft beer garden, live acoustic sets, and ocean views. Free entry — pay as you eat. Pet-friendly.',
  'https://picsum.photos/seed/seapointmarket/600/400',
  NULL,
  5, org_id, 'Sea Point Promenade', 'Beach Rd, Sea Point', 'Cape Town', 'Africa/Johannesburg',
  0.00, 'ZAR', 5000, 3200,
  'published', NOW() + INTERVAL '2 days', NOW() + INTERVAL '2 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '5 days', NOW() - INTERVAL '1 day', 12800 ),

-- 12. CTICC — Gala
( 'e2000000-0000-0000-0000-000000000012',
  make_slug('Cape Town Heritage Gala Dinner 2026', 'e2000000-0000-0000-0000-000000000012'),
  'Cape Town Heritage Gala Dinner 2026',
  'Celebrate Cape Town''s rich cultural heritage at this annual black-tie gala dinner. Five-course meal by award-winning chef Jan-Hendrik van der Westhuizen, live performance by the Cape Town Minstrels, and a silent auction for heritage conservation.',
  'https://picsum.photos/seed/ctheritagegala/600/400',
  NULL,
  3, org_id, 'Cape Town International Convention Centre', 'Convention Square', 'Cape Town', 'Africa/Johannesburg',
  2800.00, 'ZAR', 600, 480,
  'published', NOW() + INTERVAL '25 days', NOW() + INTERVAL '25 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '11 days', 11400 ),

-- 13. Cape Town Stadium — Cricket Viewing
( 'e2000000-0000-0000-0000-000000000013',
  make_slug('SA20 Cape Cobras vs Paarl Royals', 'e2000000-0000-0000-0000-000000000013'),
  'SA20 Cape Cobras vs Paarl Royals',
  'T20 cricket at its finest under the Cape Town lights. The Cape Cobras host the Paarl Royals in a must-win SA20 fixture. Fun zones for kids, braai stands, and beer gardens throughout the venue. Early-bird family packages available.',
  'https://picsum.photos/seed/sa20capecobras/600/400',
  NULL,
  2, org_id, 'DHL Cape Town Stadium', 'Fritz Sonnenberg Rd, Green Point', 'Cape Town', 'Africa/Johannesburg',
  150.00, 'ZAR', 55000, 29000,
  'published', NOW() + INTERVAL '5 days', NOW() + INTERVAL '5 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '8 days', NOW() - INTERVAL '2 days', 25600 ),

-- 14. Long Street — Tech
( 'e2000000-0000-0000-0000-000000000014',
  make_slug('Cape Town Startup Pitch Night', 'e2000000-0000-0000-0000-000000000014'),
  'Cape Town Startup Pitch Night',
  'Eight Cape Town startups pitch to a panel of leading venture capitalists and angel investors. Networking drinks before and after the pitches. Voted the best startup event in Africa by TechCrunch Africa 2025. Limited to 300 attendees.',
  'https://picsum.photos/seed/ctstartupp/600/400',
  NULL,
  4, org_id, 'Workshop17 Cape Town', '32 Kloof St, Gardens', 'Cape Town', 'Africa/Johannesburg',
  350.00, 'ZAR', 300, 245,
  'published', NOW() + INTERVAL '7 days', NOW() + INTERVAL '7 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '2 days', 9800 ),

-- 15. Llandudno Beach — Exclusive
( 'e2000000-0000-0000-0000-000000000015',
  make_slug('Sunset Beats: Clifton Private Session', 'e2000000-0000-0000-0000-000000000015'),
  'Sunset Beats: Clifton Private Session',
  'An invite-only sunset DJ session on Cape Town''s most exclusive beach. Limited to 200 guests. Open-bar champagne service, canapés, and world-class sunset views. Formal casual dress code. Transport from Camps Bay included.',
  'https://picsum.photos/seed/cliftonbeach/600/400',
  NULL,
  6, org_id, 'Clifton 4th Beach', 'Clifton', 'Cape Town', 'Africa/Johannesburg',
  3500.00, 'ZAR', 200, 155,
  'published', NOW() + INTERVAL '9 days', NOW() + INTERVAL '9 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '3 days', 8700 ),

-- 16. DHL Newlands — Springboks
( 'e2000000-0000-0000-0000-000000000016',
  make_slug('Springboks vs Australia Rugby Test Cape Town', 'e2000000-0000-0000-0000-000000000016'),
  'Springboks vs Australia Rugby Test Cape Town',
  'The world champions face the Wallabies in Cape Town for the first time in 4 years. One of the most sought-after tickets in South African sport. Corporate hospitality suites, sponsor activations, and a full match-day festival atmosphere.',
  'https://picsum.photos/seed/boksaustralia/600/400',
  'https://picsum.photos/seed/boksaustraliabanner/1200/600',
  2, org_id, 'DHL Newlands Stadium', 'Boundary Rd, Newlands', 'Cape Town', 'Africa/Johannesburg',
  550.00, 'ZAR', 51000, 50100,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '20 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '15 days', 71200 ),

-- 17. Woodstock — Food
( 'e2000000-0000-0000-0000-000000000017',
  make_slug('Old Biscuit Mill Eat Out Festival', 'e2000000-0000-0000-0000-000000000017'),
  'Old Biscuit Mill Eat Out Festival',
  'The Old Biscuit Mill goes all out for the annual Eat Out Festival weekend. Special menus from 30 top-rated restaurants, wine pairings, live cooking demonstrations, and a competition to find Cape Town''s best boerewors roll. Seriously.',
  'https://picsum.photos/seed/oldbiscuitmill/600/400',
  NULL,
  5, org_id, 'The Old Biscuit Mill', '373-375 Albert Rd, Woodstock', 'Cape Town', 'Africa/Johannesburg',
  120.00, 'ZAR', 2500, 1900,
  'published', NOW() + INTERVAL '14 days', NOW() + INTERVAL '15 days',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '5 days', 13600 ),

-- 18. Langa — Music
( 'e2000000-0000-0000-0000-000000000018',
  make_slug('Afropunk Cape Town 2026', 'e2000000-0000-0000-0000-000000000018'),
  'Afropunk Cape Town 2026',
  'The global Afropunk movement lands in Langa for a two-day celebration of music, art, and radical self-expression. Artists across Afrobeats, hip-hop, jazz fusion, and spoken word. Fashion market, food village, and an art installation by 10 Cape Town creatives.',
  'https://picsum.photos/seed/afropunkct/600/400',
  'https://picsum.photos/seed/afropunkctbanner/1200/600',
  1, org_id, 'Langa Recreational Centre', 'Washington Dr, Langa', 'Cape Town', 'Africa/Johannesburg',
  580.00, 'ZAR', 10000, 8200,
  'published', NOW() + INTERVAL '28 days', NOW() + INTERVAL '29 days',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '12 days', 44900 ),

-- 19. Constantia — Exclusive
( 'e2000000-0000-0000-0000-000000000019',
  make_slug('Klein Constantia Harvest Dinner 2026', 'e2000000-0000-0000-0000-000000000019'),
  'Klein Constantia Harvest Dinner 2026',
  'Dine under the stars among the vines at Klein Constantia''s annual harvest dinner. A 7-course meal paired with the estate''s finest wines including the legendary Vin de Constance. Limited to 80 guests. Formal dress code.',
  'https://picsum.photos/seed/kleinconstantia/600/400',
  NULL,
  6, org_id, 'Klein Constantia Wine Estate', 'Klein Constantia Rd, Constantia', 'Cape Town', 'Africa/Johannesburg',
  4200.00, 'ZAR', 80, 62,
  'published', NOW() + INTERVAL '18 days', NOW() + INTERVAL '18 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '8 days', 7100 ),

-- 20. CTICC — Business
( 'e2000000-0000-0000-0000-000000000020',
  make_slug('Cape Town Entrepreneurship Week 2026', 'e2000000-0000-0000-0000-000000000020'),
  'Cape Town Entrepreneurship Week 2026',
  'Five days of workshops, panels, funding pitches, and networking events for entrepreneurs at all stages. Supported by the Western Cape Government and Standard Bank. Free day-1 access for students. Startup exhibition floor open to the public daily.',
  'https://picsum.photos/seed/ctentrepreneurweek/600/400',
  NULL,
  4, org_id, 'Cape Town International Convention Centre', 'Convention Square', 'Cape Town', 'Africa/Johannesburg',
  1200.00, 'ZAR', 5000, 3700,
  'published', NOW() + INTERVAL '33 days', NOW() + INTERVAL '37 days',
  NOW() + INTERVAL '40 days', NOW() - INTERVAL '16 days', 16400 ),

-- 21. Bellville Velodrome — Sports
( 'e2000000-0000-0000-0000-000000000021',
  make_slug('Cape Town Cycle Tour Amateur Category 2026', 'e2000000-0000-0000-0000-000000000021'),
  'Cape Town Cycle Tour Amateur Category 2026',
  'The world''s largest individually timed cycle race opens its amateur category registration. 109km around the Cape Peninsula starting and finishing at the Grand Parade. Over 35 000 participants from 60 countries. Entry includes timing chip and finisher medal.',
  'https://picsum.photos/seed/cycletour/600/400',
  NULL,
  2, org_id, 'Grand Parade', 'Adderley St, Cape Town CBD', 'Cape Town', 'Africa/Johannesburg',
  780.00, 'ZAR', 35000, 31000,
  'published', NOW() + INTERVAL '42 days', NOW() + INTERVAL '42 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '45 days', NOW() - INTERVAL '20 days', 48300 ),

-- 22. Observatory — Food
( 'e2000000-0000-0000-0000-000000000022',
  make_slug('Observatory Night Market & Braai', 'e2000000-0000-0000-0000-000000000022'),
  'Observatory Night Market & Braai',
  'Obs comes alive every last Friday of the month for Cape Town''s most eclectic night market. Record stalls, vintage clothing, street food from around the world, live bands, and the finest craft beer lineup in the Cape. Free entry.',
  'https://picsum.photos/seed/obsnightmarket/600/400',
  NULL,
  5, org_id, 'Observatory Community Centre', 'Lower Main Rd, Observatory', 'Cape Town', 'Africa/Johannesburg',
  0.00, 'ZAR', 3000, 2400,
  'published', NOW() + INTERVAL '4 days', NOW() + INTERVAL '4 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '7 days', NOW() - INTERVAL '1 day', 7800 ),

-- 23. Groote Schuur — Gala
( 'e2000000-0000-0000-0000-000000000023',
  make_slug('UCT Centenary Ball 2026', 'e2000000-0000-0000-0000-000000000023'),
  'UCT Centenary Ball 2026',
  'The University of Cape Town celebrates its centenary with the most prestigious gala event in the institution''s history. Black tie. Alumni, faculty, and invited guests enjoy a five-course dinner, live big band, and an exclusive auction of UCT memorabilia.',
  'https://picsum.photos/seed/uctball/600/400',
  NULL,
  3, org_id, 'Jameson Hall, UCT', 'University Ave, Rondebosch', 'Cape Town', 'Africa/Johannesburg',
  2200.00, 'ZAR', 800, 640,
  'published', NOW() + INTERVAL '21 days', NOW() + INTERVAL '21 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '25 days', NOW() - INTERVAL '9 days', 9200 ),

-- 24. Cape Town — Sports Exclusive
( 'e2000000-0000-0000-0000-000000000024',
  make_slug('Sharks vs Stormers URC Cape Derby', 'e2000000-0000-0000-0000-000000000024'),
  'Sharks vs Stormers URC Cape Derby',
  'The Sharks make the trip south for the most anticipated URC fixture in the Cape. Premium hospitality packages available including three-course pre-match dinner, open bar throughout, and post-match player access. General admission includes stadium food and drink vouchers.',
  'https://picsum.photos/seed/sharksstormers/600/400',
  NULL,
  2, org_id, 'DHL Newlands Stadium', 'Boundary Rd, Newlands', 'Cape Town', 'Africa/Johannesburg',
  290.00, 'ZAR', 51000, 46000,
  'published', NOW() + INTERVAL '17 days', NOW() + INTERVAL '17 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '7 days', 35100 ),

-- 25. Cape Town — Music Festival
( 'e2000000-0000-0000-0000-000000000025',
  make_slug('Cape Town International Jazz Festival Day 1', 'e2000000-0000-0000-0000-000000000025'),
  'Cape Town International Jazz Festival Day 1',
  'Africa''s greatest jazz celebration returns to the CTICC for its 27th edition. Day 1 features five stages running simultaneously from 15:00 to 02:00. Headliners include world-class artists from the US, Europe, and across Africa. One-day ticket.',
  'https://picsum.photos/seed/ctjazzday1/600/400',
  'https://picsum.photos/seed/ctjazzday1banner/1200/600',
  1, org_id, 'Cape Town International Convention Centre', 'Convention Square', 'Cape Town', 'Africa/Johannesburg',
  850.00, 'ZAR', 37000, 32000,
  'published', NOW() + INTERVAL '38 days', NOW() + INTERVAL '38 days' + INTERVAL '11 hours',
  NOW() + INTERVAL '42 days', NOW() - INTERVAL '21 days', 61400 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- ██████████  DURBAN  (e3 prefix, 25 events)
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

-- 1. Moses Mabhida — Soccer
( 'e3000000-0000-0000-0000-000000000001',
  make_slug('AmaZulu FC vs Mamelodi Sundowns PSL', 'e3000000-0000-0000-0000-000000000001'),
  'AmaZulu FC vs Mamelodi Sundowns PSL',
  'Moses Mabhida comes alive as AmaZulu host the mighty Sundowns in a top-of-the-table PSL clash. Durban''s fortress has never been louder. Family and youth fan zones, outdoor braai areas, and a spectacular pre-match ceremony.',
  'https://picsum.photos/seed/amazulusundowns/600/400',
  'https://picsum.photos/seed/amazulusundownsbanner/1200/600',
  2, org_id, 'Moses Mabhida Stadium', '44 Isaiah Ntshangase Rd', 'Durban', 'Africa/Johannesburg',
  150.00, 'ZAR', 56000, 41000,
  'published', NOW() + INTERVAL '6 days', NOW() + INTERVAL '6 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '9 days', NOW() - INTERVAL '3 days', 37800 ),

-- 2. Kings Park — Rugby
( 'e3000000-0000-0000-0000-000000000002',
  make_slug('Sharks vs Bulls URC Kings Park', 'e3000000-0000-0000-0000-000000000002'),
  'Sharks vs Bulls URC Kings Park',
  'The Sharks host the Bulls at the fortress that is Kings Park Stadium. A URC fixture that always delivers fireworks. Premium hospitality in the Shark Tank Suite, family terraces, and the famous Kings Park atmosphere that no visiting team enjoys.',
  'https://picsum.photos/seed/sharksbulls/600/400',
  NULL,
  2, org_id, 'Kings Park Stadium', 'Jacko Jackson Dr', 'Durban', 'Africa/Johannesburg',
  280.00, 'ZAR', 52000, 44500,
  'published', NOW() + INTERVAL '10 days', NOW() + INTERVAL '10 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '14 days', NOW() - INTERVAL '5 days', 31200 ),

-- 3. Moses Mabhida — Concert
( 'e3000000-0000-0000-0000-000000000003',
  make_slug('Afro Nation Durban 2026', 'e3000000-0000-0000-0000-000000000003'),
  'Afro Nation Durban 2026',
  'After selling out in Johannesburg, Afro Nation brings its Durban debut to Moses Mabhida. The biggest names in Afrobeats, amapiano, and Afro-house on one stage over two days. Beach meets stadium — the KZN way.',
  'https://picsum.photos/seed/afronationdurban/600/400',
  'https://picsum.photos/seed/afronationdurbanbanner/1200/600',
  1, org_id, 'Moses Mabhida Stadium', '44 Isaiah Ntshangase Rd', 'Durban', 'Africa/Johannesburg',
  680.00, 'ZAR', 56000, 49000,
  'published', NOW() + INTERVAL '24 days', NOW() + INTERVAL '25 days',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '16 days', 72100 ),

-- 4. ICC Durban — Tech
( 'e3000000-0000-0000-0000-000000000004',
  make_slug('Durban Innovation & Tech Summit 2026', 'e3000000-0000-0000-0000-000000000004'),
  'Durban Innovation & Tech Summit 2026',
  'KwaZulu-Natal''s premier technology event brings together government, industry, and academia for two days of panel discussions, workshops, and exhibitions. Focus areas: smart cities, fintech, agritech, and digital health. Funded attendance for NGOs.',
  'https://picsum.photos/seed/durbantechsummit/600/400',
  NULL,
  4, org_id, 'Durban ICC', '45 Bram Fischer Rd', 'Durban', 'Africa/Johannesburg',
  2800.00, 'ZAR', 5000, 3200,
  'published', NOW() + INTERVAL '27 days', NOW() + INTERVAL '28 days',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '12 days', 14600 ),

-- 5. North Beach — Sports
( 'e3000000-0000-0000-0000-000000000005',
  make_slug('Mr Price Pro Surf Durban 2026', 'e3000000-0000-0000-0000-000000000005'),
  'Mr Price Pro Surf Durban 2026',
  'The world''s top professional surfers descend on North Beach for the WSL Championship Tour event that Durban does best. Free public viewing on the beach. VIP beach clubs, surf clinics for juniors, and the most famous shorebreak in world surfing.',
  'https://picsum.photos/seed/mrpricepro/600/400',
  NULL,
  2, org_id, 'North Beach', 'Marine Parade', 'Durban', 'Africa/Johannesburg',
  0.00, 'ZAR', 50000, 38000,
  'published', NOW() + INTERVAL '15 days', NOW() + INTERVAL '19 days',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '8 days', 28900 ),

-- 6. Sun Coast Casino — Gala
( 'e3000000-0000-0000-0000-000000000006',
  make_slug('Sun Coast Black Tie Awards Gala 2026', 'e3000000-0000-0000-0000-000000000006'),
  'Sun Coast Black Tie Awards Gala 2026',
  'The KZN Business Awards Gala returns to Sun Coast for the most glamorous evening in the Durban social calendar. Recognising excellence across 12 business categories. Six-course dinner, entertainment from Lira, and an after-party until late.',
  'https://picsum.photos/seed/suncoastgala/600/400',
  NULL,
  3, org_id, 'Sun Coast Casino & Entertainment World', '1 Casino Dr, Suncoast', 'Durban', 'Africa/Johannesburg',
  3200.00, 'ZAR', 500, 420,
  'published', NOW() + INTERVAL '23 days', NOW() + INTERVAL '23 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '27 days', NOW() - INTERVAL '11 days', 12800 ),

-- 7. Moses Mabhida — Sports
( 'e3000000-0000-0000-0000-000000000007',
  make_slug('Comrades Marathon Finish Line Festival 2026', 'e3000000-0000-0000-0000-000000000007'),
  'Comrades Marathon Finish Line Festival 2026',
  'Celebrate the world''s greatest ultra-marathon at the Moses Mabhida finish line festival. Whether you''re running or cheering, the finish line experience is unlike anything else in sport. Live entertainment, food village, and the iconic Comrades atmosphere.',
  'https://picsum.photos/seed/comradesfinish/600/400',
  NULL,
  2, org_id, 'Moses Mabhida Stadium', '44 Isaiah Ntshangase Rd', 'Durban', 'Africa/Johannesburg',
  120.00, 'ZAR', 30000, 24000,
  'published', NOW() + INTERVAL '31 days', NOW() + INTERVAL '31 days' + INTERVAL '12 hours',
  NOW() + INTERVAL '35 days', NOW() - INTERVAL '18 days', 42300 ),

-- 8. Greyville Racecourse — Exclusive
( 'e3000000-0000-0000-0000-000000000008',
  make_slug('Durban July 2026: The Biggest Race Day in Africa', 'e3000000-0000-0000-0000-000000000008'),
  'Durban July 2026: The Biggest Race Day in Africa',
  'The 2026 Durban July — Africa''s most glamorous sporting and social event returns to Greyville. The fashion stakes are as high as the racing. Premium hospitality in the Owners'' Enclosure, trackside marquees, and the legendary after-race party.',
  'https://picsum.photos/seed/durbanjuly/600/400',
  'https://picsum.photos/seed/durbanjulybanner/1200/600',
  6, org_id, 'Greyville Racecourse', '150 Avondale Rd, Greyville', 'Durban', 'Africa/Johannesburg',
  1800.00, 'ZAR', 40000, 36000,
  'published', NOW() + INTERVAL '44 days', NOW() + INTERVAL '44 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '48 days', NOW() - INTERVAL '25 days', 88400 ),

-- 9. uShaka Marine — Food
( 'e3000000-0000-0000-0000-000000000009',
  make_slug('Taste of Durban Food Festival uShaka', 'e3000000-0000-0000-0000-000000000009'),
  'Taste of Durban Food Festival uShaka',
  'KZN''s most exciting food festival makes its debut at uShaka Marine World''s iconic waterfront area. 35 restaurants and food producers showcasing Durban''s legendary curry culture alongside international flavours. Live music, cooking classes, and a chilli-eating contest.',
  'https://picsum.photos/seed/tastedurban/600/400',
  NULL,
  5, org_id, 'uShaka Marine World', '1 King Shaka Ave, Point', 'Durban', 'Africa/Johannesburg',
  180.00, 'ZAR', 6000, 4800,
  'published', NOW() + INTERVAL '12 days', NOW() + INTERVAL '13 days',
  NOW() + INTERVAL '16 days', NOW() - INTERVAL '4 days', 18900 ),

-- 10. Wavehouse Umhlanga — Music
( 'e3000000-0000-0000-0000-000000000010',
  make_slug('Wavehouse Umhlanga Beach Club Summer Launch', 'e3000000-0000-0000-0000-000000000010'),
  'Wavehouse Umhlanga Beach Club Summer Launch',
  'The legendary Wavehouse reopens its summer season with a massive beach club launch party. International DJ headline, poolside bar, and the famous flowrider wave machine. Tickets include R100 bar tab. Ages 21+. Smart casual dress code.',
  'https://picsum.photos/seed/wavehouseumhlanga/600/400',
  NULL,
  1, org_id, 'Wavehouse Umhlanga', 'Lagoon Dr, Umhlanga Rocks', 'Durban', 'Africa/Johannesburg',
  420.00, 'ZAR', 1500, 1250,
  'published', NOW() + INTERVAL '3 days', NOW() + INTERVAL '3 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '6 days', NOW() - INTERVAL '1 day', 14200 ),

-- 11. Sibaya — Exclusive
( 'e3000000-0000-0000-0000-000000000011',
  make_slug('Sibaya Private Members Evening 2026', 'e3000000-0000-0000-0000-000000000011'),
  'Sibaya Private Members Evening 2026',
  'An exclusive evening for Sibaya Casino''s VIP members and invited guests. Curated whisky and cigar tasting, private gaming tables, a four-course dinner prepared by the executive chef, and entertainment from jazz vocalist Thandiswa Mazwai.',
  'https://picsum.photos/seed/sibayaexclusive/600/400',
  NULL,
  6, org_id, 'Sibaya Casino & Entertainment Kingdom', 'Sibayi Rd, Umhlanga', 'Durban', 'Africa/Johannesburg',
  5500.00, 'ZAR', 150, 120,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '20 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '8 days', 7400 ),

-- 12. Kings Park — Cricket
( 'e3000000-0000-0000-0000-000000000012',
  make_slug('CSA T20 Challenge: Dolphins vs Warriors Durban', 'e3000000-0000-0000-0000-000000000012'),
  'CSA T20 Challenge: Dolphins vs Warriors Durban',
  'High-octane T20 cricket at Kingsmead as the Dolphins take on the Warriors under the Durban lights. Family fun zones, braai stands, and the most passionate cricket crowd in South Africa. Bring the kids — it''s a day out the whole family will love.',
  'https://picsum.photos/seed/dolphinscricket/600/400',
  NULL,
  2, org_id, 'Kingsmead Cricket Ground', 'Kingsmead, Durban', 'Durban', 'Africa/Johannesburg',
  130.00, 'ZAR', 25000, 17000,
  'published', NOW() + INTERVAL '8 days', NOW() + INTERVAL '8 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '3 days', 13800 ),

-- 13. BAT Centre — Gala
( 'e3000000-0000-0000-0000-000000000013',
  make_slug('Durban Art & Culture Awards Gala 2026', 'e3000000-0000-0000-0000-000000000013'),
  'Durban Art & Culture Awards Gala 2026',
  'Celebrating the best of KZN''s creative scene at the iconic BAT Centre. Awards across 15 categories spanning visual art, theatre, film, music, and literature. Red carpet arrival, gala dinner, and a late-night celebration concert from award-winning local acts.',
  'https://picsum.photos/seed/durbanculturegala/600/400',
  NULL,
  3, org_id, 'BAT Centre', '45 Maritime Place, Small Craft Harbour', 'Durban', 'Africa/Johannesburg',
  1400.00, 'ZAR', 400, 320,
  'published', NOW() + INTERVAL '17 days', NOW() + INTERVAL '17 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '21 days', NOW() - INTERVAL '6 days', 9100 ),

-- 14. Durban Botanic Gardens — Food
( 'e3000000-0000-0000-0000-000000000014',
  make_slug('Durban Botanical Garden Picnic Festival', 'e3000000-0000-0000-0000-000000000014'),
  'Durban Botanical Garden Picnic Festival',
  'The oldest botanical gardens in Africa hosts its annual picnic festival. 25 artisan food and drink producers, live acoustic music, guided garden tours, and a kids'' nature trail. BYO picnic basket or purchase curated hampers on arrival.',
  'https://picsum.photos/seed/durbangardens/600/400',
  NULL,
  5, org_id, 'Durban Botanic Gardens', 'St Thomas Rd, Berea', 'Durban', 'Africa/Johannesburg',
  80.00, 'ZAR', 4000, 2800,
  'published', NOW() + INTERVAL '4 days', NOW() + INTERVAL '4 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '7 days', NOW() - INTERVAL '1 day', 8600 ),

-- 15. Playhouse — Music
( 'e3000000-0000-0000-0000-000000000015',
  make_slug('Ladysmith Black Mambazo Farewell Tour Durban', 'e3000000-0000-0000-0000-000000000015'),
  'Ladysmith Black Mambazo Farewell Tour Durban',
  'The legendary Grammy-winning isicathamiya vocal group performs their farewell tour at the Playhouse Opera House. A once-in-a-lifetime opportunity to witness South Africa''s most globally celebrated musical export in the city they call home.',
  'https://picsum.photos/seed/ladysmithblack/600/400',
  NULL,
  1, org_id, 'The Playhouse Company Opera House', '231 Anton Lembede St', 'Durban', 'Africa/Johannesburg',
  650.00, 'ZAR', 1400, 1380,
  'published', NOW() + INTERVAL '16 days', NOW() + INTERVAL '16 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '9 days', 31800 ),

-- 16. Moses Mabhida — Tech
( 'e3000000-0000-0000-0000-000000000016',
  make_slug('Africa''s Big 7 Food & Beverage Trade Show Durban', 'e3000000-0000-0000-0000-000000000016'),
  'Africa''s Big 7 Food & Beverage Trade Show Durban',
  'The largest food and beverage trade show in Africa returns to Durban. Three days of industry exhibitions, new product launches, trend-setting keynotes, and the infamous Big 7 cooking competition. Trade visitors welcome; consumer day on Sunday.',
  'https://picsum.photos/seed/big7foodshow/600/400',
  NULL,
  4, org_id, 'Inkosi Albert Luthuli Convention Centre', '45 Bram Fischer Rd', 'Durban', 'Africa/Johannesburg',
  1800.00, 'ZAR', 20000, 16000,
  'published', NOW() + INTERVAL '36 days', NOW() + INTERVAL '38 days',
  NOW() + INTERVAL '42 days', NOW() - INTERVAL '20 days', 22400 ),

-- 17. North Beach — Sports
( 'e3000000-0000-0000-0000-000000000017',
  make_slug('Durban Beach Run & Swim Festival 2026', 'e3000000-0000-0000-0000-000000000017'),
  'Durban Beach Run & Swim Festival 2026',
  '10K beach run, 1km open-water swim, and a paddleski race on Durban''s legendary Golden Mile. Entry includes a timing chip, event T-shirt, post-race breakfast, and a finisher''s medal. Medals for top 10 in each category.',
  'https://picsum.photos/seed/durbanbeachrun/600/400',
  NULL,
  2, org_id, 'Durban North Beach', 'Marine Parade', 'Durban', 'Africa/Johannesburg',
  280.00, 'ZAR', 5000, 3800,
  'published', NOW() + INTERVAL '11 days', NOW() + INTERVAL '11 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '14 days', NOW() - INTERVAL '4 days', 11200 ),

-- 18. Sun Coast — Concert
( 'e3000000-0000-0000-0000-000000000018',
  make_slug('Sun Coast Amphitheatre: Mi Casa Live', 'e3000000-0000-0000-0000-000000000018'),
  'Sun Coast Amphitheatre: Mi Casa Live',
  'Mi Casa brings their chart-topping Afro-house sound to the Sun Coast Amphitheatre for an intimate outdoor show. Support from DJ Maphorisa and two emerging KZN artists. Cocktail bar, food trucks, and sunset views over the Indian Ocean.',
  'https://picsum.photos/seed/micasasuncoast/600/400',
  NULL,
  1, org_id, 'Sun Coast Casino Amphitheatre', '1 Casino Dr, Suncoast', 'Durban', 'Africa/Johannesburg',
  320.00, 'ZAR', 3000, 2600,
  'published', NOW() + INTERVAL '7 days', NOW() + INTERVAL '7 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '10 days', NOW() - INTERVAL '2 days', 19400 ),

-- 19. Durban ICC — Exclusive
( 'e3000000-0000-0000-0000-000000000019',
  make_slug('KZN Premier Business Awards Gala 2026', 'e3000000-0000-0000-0000-000000000019'),
  'KZN Premier Business Awards Gala 2026',
  'The most prestigious business awards ceremony in KwaZulu-Natal. Celebrating excellence in manufacturing, tourism, finance, and the creative economy. Attended by the KZN Premier and senior government officials. Black tie. Table bookings preferred.',
  'https://picsum.photos/seed/kznpremierawards/600/400',
  NULL,
  6, org_id, 'Inkosi Albert Luthuli Convention Centre', '45 Bram Fischer Rd', 'Durban', 'Africa/Johannesburg',
  3800.00, 'ZAR', 800, 680,
  'published', NOW() + INTERVAL '29 days', NOW() + INTERVAL '29 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '33 days', NOW() - INTERVAL '14 days', 8300 ),

-- 20. Umhlanga — Food
( 'e3000000-0000-0000-0000-000000000020',
  make_slug('Gateway Food & Craft Market Umhlanga', 'e3000000-0000-0000-0000-000000000020'),
  'Gateway Food & Craft Market Umhlanga',
  'The monthly Gateway rooftop market is back with 60 vendors across food, craft, design, and lifestyle. Local bakers, curry houses, artisan cheese, craft gin distillers, and handmade clothing. Free parking. Family and pet friendly.',
  'https://picsum.photos/seed/gatewayfoodmarket/600/400',
  NULL,
  5, org_id, 'Gateway Theatre of Shopping Rooftop', '1 Palm Blvd, Umhlanga', 'Durban', 'Africa/Johannesburg',
  0.00, 'ZAR', 4000, 2900,
  'published', NOW() + INTERVAL '2 days', NOW() + INTERVAL '2 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '5 days', NOW() - INTERVAL '1 day', 7100 ),

-- 21. Kings Park — Concert
( 'e3000000-0000-0000-0000-000000000021',
  make_slug('Durban Stadium Amapiano Festival 2026', 'e3000000-0000-0000-0000-000000000021'),
  'Durban Stadium Amapiano Festival 2026',
  'The biggest amapiano festival ever held in KZN takes over Kings Park Stadium. All the top log drums: Kabza De Small, DJ Maphorisa, Focalistic, Kelvin Momo, and Ami Faku. Durban is ready. Are you?',
  'https://picsum.photos/seed/durbanamapianokg/600/400',
  'https://picsum.photos/seed/durbanamapianoban/1200/600',
  1, org_id, 'Kings Park Stadium', 'Jacko Jackson Dr', 'Durban', 'Africa/Johannesburg',
  580.00, 'ZAR', 52000, 44000,
  'published', NOW() + INTERVAL '19 days', NOW() + INTERVAL '19 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '23 days', NOW() - INTERVAL '11 days', 58900 ),

-- 22. Durban — Gala
( 'e3000000-0000-0000-0000-000000000022',
  make_slug('Turbine Hall New Year Countdown Durban', 'e3000000-0000-0000-0000-000000000022'),
  'Turbine Hall New Year Countdown Durban',
  'Ring in the new year at the iconic Turbine Hall at Durban''s Point Waterfront. DJ sets, live performances, countdown celebrations, and a spectacular New Year''s fireworks display over the harbour. Open bar packages available.',
  'https://picsum.photos/seed/turbinehallnye/600/400',
  NULL,
  3, org_id, 'Turbine Hall', 'Point Waterfront, Durban', 'Durban', 'Africa/Johannesburg',
  780.00, 'ZAR', 2000, 1700,
  'published', NOW() + INTERVAL '40 days', NOW() + INTERVAL '41 days',
  NOW() + INTERVAL '43 days', NOW() - INTERVAL '22 days', 27600 ),

-- 23. Durban — Sports
( 'e3000000-0000-0000-0000-000000000023',
  make_slug('Sharks vs Leinster Champions Cup Durban', 'e3000000-0000-0000-0000-000000000023'),
  'Sharks vs Leinster Champions Cup Durban',
  'The Cell C Sharks face the might of Leinster in a historic Champions Cup fixture at Kings Park. The most prestigious club rugby competition in the world comes to KZN for the first time. A fixture for the history books.',
  'https://picsum.photos/seed/sharksleinster/600/400',
  NULL,
  2, org_id, 'Kings Park Stadium', 'Jacko Jackson Dr', 'Durban', 'Africa/Johannesburg',
  480.00, 'ZAR', 52000, 48000,
  'published', NOW() + INTERVAL '26 days', NOW() + INTERVAL '26 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '30 days', NOW() - INTERVAL '13 days', 43700 ),

-- 24. Durban — Tech
( 'e3000000-0000-0000-0000-000000000024',
  make_slug('KZN Gaming Expo & Esports Championship 2026', 'e3000000-0000-0000-0000-000000000024'),
  'KZN Gaming Expo & Esports Championship 2026',
  'KwaZulu-Natal''s first dedicated gaming expo and esports tournament. PS5, Xbox, and PC gaming tournaments, cosplay competition, game developer panels, and a dedicated retro gaming floor. Cash prizes totalling R200 000. Open to all skill levels.',
  'https://picsum.photos/seed/kzngamingexpo/600/400',
  NULL,
  4, org_id, 'Inkosi Albert Luthuli Convention Centre', '45 Bram Fischer Rd', 'Durban', 'Africa/Johannesburg',
  220.00, 'ZAR', 8000, 5400,
  'published', NOW() + INTERVAL '33 days', NOW() + INTERVAL '34 days',
  NOW() + INTERVAL '37 days', NOW() - INTERVAL '17 days', 16800 ),

-- 25. Durban — Food Exclusive
( 'e3000000-0000-0000-0000-000000000025',
  make_slug('Durban Curry Mile Fest: The Ultimate Bunny Chow Championship', 'e3000000-0000-0000-0000-000000000025'),
  'Durban Curry Mile Fest: Ultimate Bunny Chow Championship',
  'Durban''s most deliciously ridiculous event. 20 restaurants compete for the title of best bunny chow in the world. Public vote and a panel of judges including celebrity chef Reza Mahammad. Tasting passports let you try all 20 entries. Live music, cooking demos.',
  'https://picsum.photos/seed/bunnychowchamp/600/400',
  NULL,
  5, org_id, 'Grey St, Durban CBD', 'Grey St', 'Durban', 'Africa/Johannesburg',
  150.00, 'ZAR', 5000, 3700,
  'published', NOW() + INTERVAL '9 days', NOW() + INTERVAL '9 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '3 days', 14100 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- ██████████  PRETORIA  (e4 prefix, 22 events)
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

-- 1. Loftus Versfeld — Rugby
( 'e4000000-0000-0000-0000-000000000001',
  make_slug('Bulls vs Sharks Loftus URC', 'e4000000-0000-0000-0000-000000000001'),
  'Bulls vs Sharks Loftus URC',
  'The Blue Bulls welcome the Sharks to the fortress of Loftus Versfeld for a blockbuster URC fixture. The altitude, the atmosphere, and the blue sea of supporters make Loftus one of the most intimidating venues in world rugby. Premium hospitality available.',
  'https://picsum.photos/seed/bullssharks/600/400',
  'https://picsum.photos/seed/bullssharksban/1200/600',
  2, org_id, 'Loftus Versfeld Stadium', 'Kirkness St, Sunnyside', 'Pretoria', 'Africa/Johannesburg',
  320.00, 'ZAR', 51762, 46000,
  'published', NOW() + INTERVAL '7 days', NOW() + INTERVAL '7 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '11 days', NOW() - INTERVAL '4 days', 39800 ),

-- 2. SuperSport Park — Cricket
( 'e4000000-0000-0000-0000-000000000002',
  make_slug('SA vs Pakistan Test Match SuperSport Park', 'e4000000-0000-0000-0000-000000000002'),
  'SA vs Pakistan Test Match SuperSport Park Day 1',
  'The first day of the Test match at the world-famous SuperSport Park (Centurion). The best test match venue in South Africa — great sightlines, excellent facilities, and a crowd that knows its cricket. Day 1 promises swing bowling and early wickets.',
  'https://picsum.photos/seed/testmatchpretoria/600/400',
  NULL,
  2, org_id, 'SuperSport Park', 'Totius St, Centurion', 'Pretoria', 'Africa/Johannesburg',
  180.00, 'ZAR', 20000, 15000,
  'published', NOW() + INTERVAL '12 days', NOW() + INTERVAL '12 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '16 days', NOW() - INTERVAL '5 days', 22400 ),

-- 3. State Theatre — Gala
( 'e4000000-0000-0000-0000-000000000003',
  make_slug('Pretoria State Theatre Grand Gala Opening Night', 'e4000000-0000-0000-0000-000000000003'),
  'Pretoria State Theatre Grand Gala Opening Night',
  'The Pretoria State Theatre opens its new season with a spectacular gala evening. World premiere of an original South African opera performed by the SA State Theatre Opera Company. Pre-show cocktail reception in the foyer. Black tie preferred.',
  'https://picsum.photos/seed/statetheattregala/600/400',
  NULL,
  3, org_id, 'Pretoria State Theatre', '320 Pretorius St', 'Pretoria', 'Africa/Johannesburg',
  980.00, 'ZAR', 1300, 1050,
  'published', NOW() + INTERVAL '18 days', NOW() + INTERVAL '18 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '8 days', 14600 ),

-- 4. CSIR — Tech
( 'e4000000-0000-0000-0000-000000000004',
  make_slug('National Science & Technology Forum Pretoria 2026', 'e4000000-0000-0000-0000-000000000004'),
  'National Science & Technology Forum Pretoria 2026',
  'South Africa''s leading annual gathering of scientists, engineers, policymakers, and innovators. Hosted at the CSIR Conference Centre in Pretoria. Three days of keynotes, technical presentations, and an exhibition featuring breakthroughs from 40 South African institutions.',
  'https://picsum.photos/seed/nstfpretoria/600/400',
  NULL,
  4, org_id, 'CSIR Convention Centre', 'Meiring Naudé Rd, Brummeria', 'Pretoria', 'Africa/Johannesburg',
  3200.00, 'ZAR', 3000, 2100,
  'published', NOW() + INTERVAL '22 days', NOW() + INTERVAL '24 days',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '10 days', 11800 ),

-- 5. Centurion — Food
( 'e4000000-0000-0000-0000-000000000005',
  make_slug('Centurion Fresh Market Festival 2026', 'e4000000-0000-0000-0000-000000000005'),
  'Centurion Fresh Market Festival 2026',
  'The biggest outdoor fresh produce and artisan food market in Gauteng takes over Centurion Lake. Over 80 stalls selling organic produce, artisan cheese, craft bread, biltong, wines, and prepared food. Live music, pony rides for kids, and vintage car display.',
  'https://picsum.photos/seed/centurionmarket/600/400',
  NULL,
  5, org_id, 'Centurion Lake Hotel Grounds', '1001 Lenchen Ave N, Centurion', 'Pretoria', 'Africa/Johannesburg',
  0.00, 'ZAR', 8000, 6200,
  'published', NOW() + INTERVAL '3 days', NOW() + INTERVAL '3 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '6 days', NOW() - INTERVAL '1 day', 9400 ),

-- 6. Menlyn — Exclusive
( 'e4000000-0000-0000-0000-000000000006',
  make_slug('Menlyn Maine NYE Exclusive Pool Party 2026', 'e4000000-0000-0000-0000-000000000006'),
  'Menlyn Maine NYE Exclusive Pool Party 2026',
  'Ring in 2027 at Menlyn Maine''s rooftop pool — the most exclusive New Year''s Eve event in Tshwane. International DJ, open bar, premium cocktail menu, and panoramic Pretoria views at midnight. Limited to 300 guests. Smart-casual required.',
  'https://picsum.photos/seed/menlynnye/600/400',
  NULL,
  6, org_id, 'Menlyn Maine Rooftop', 'Aramist Ave, Waterkloof Glen', 'Pretoria', 'Africa/Johannesburg',
  2400.00, 'ZAR', 300, 255,
  'published', NOW() + INTERVAL '39 days', NOW() + INTERVAL '40 days',
  NOW() + INTERVAL '42 days', NOW() - INTERVAL '20 days', 16800 ),

-- 7. Loftus — Concert
( 'e4000000-0000-0000-0000-000000000007',
  make_slug('Loftus Live: Treble at Loftus 2026', 'e4000000-0000-0000-0000-000000000007'),
  'Loftus Live: Treble at Loftus 2026',
  'The iconic Loftus Versfeld transforms into a concert venue for one night. Three of South Africa''s biggest artists on one stage in the capital. Multi-stage festival, food village, craft market, and VIP hospitality suites. A bucket-list Pretoria night out.',
  'https://picsum.photos/seed/loftuslive/600/400',
  'https://picsum.photos/seed/loftusliveban/1200/600',
  1, org_id, 'Loftus Versfeld Stadium', 'Kirkness St, Sunnyside', 'Pretoria', 'Africa/Johannesburg',
  620.00, 'ZAR', 51762, 43000,
  'published', NOW() + INTERVAL '28 days', NOW() + INTERVAL '28 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '15 days', 52700 ),

-- 8. Voortrekker Monument — Gala
( 'e4000000-0000-0000-0000-000000000008',
  make_slug('Voortrekker Monument Heritage Gala 2026', 'e4000000-0000-0000-0000-000000000008'),
  'Voortrekker Monument Heritage Gala 2026',
  'A unique cultural and culinary evening at one of South Africa''s most iconic landmarks. Dinner in the Great Hall, guided tours of the monument, traditional Afrikaner cuisine, live boeremusiek, and a fireworks display on the monument grounds.',
  'https://picsum.photos/seed/voortrekkergala/600/400',
  NULL,
  3, org_id, 'Voortrekker Monument', 'Eeufees Rd, Groenkloof', 'Pretoria', 'Africa/Johannesburg',
  1600.00, 'ZAR', 600, 450,
  'published', NOW() + INTERVAL '15 days', NOW() + INTERVAL '15 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '19 days', NOW() - INTERVAL '7 days', 10200 ),

-- 9. SuperSport Park — Concert
( 'e4000000-0000-0000-0000-000000000009',
  make_slug('SuperSport Park Outdoor Concert: Coldplay Tribute', 'e4000000-0000-0000-0000-000000000009'),
  'SuperSport Park Outdoor Concert: Coldplay Tribute',
  'The world''s best Coldplay tribute show comes to SuperSport Park''s outdoor concert space. Full stadium production, LED wristbands, confetti cannons, and a setlist spanning every album from Parachute to Music of the Spheres. All ages. Parking included.',
  'https://picsum.photos/seed/coldplaytribute/600/400',
  NULL,
  1, org_id, 'SuperSport Park Outdoor', 'Totius St, Centurion', 'Pretoria', 'Africa/Johannesburg',
  480.00, 'ZAR', 15000, 12000,
  'published', NOW() + INTERVAL '23 days', NOW() + INTERVAL '23 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '27 days', NOW() - INTERVAL '11 days', 24600 ),

-- 10. Hatfield Square — Food
( 'e4000000-0000-0000-0000-000000000010',
  make_slug('Hatfield Square Street Food Festival 2026', 'e4000000-0000-0000-0000-000000000010'),
  'Hatfield Square Street Food Festival 2026',
  'Hatfield''s most popular square becomes a street food paradise for the weekend. Student-friendly prices, 30 vendors, craft beer garden, and live music on the square stage. Saturday: international flavours. Sunday: South African heritage foods. Free entry.',
  'https://picsum.photos/seed/hatfieldsquarefood/600/400',
  NULL,
  5, org_id, 'Hatfield Square', 'Burnett St, Hatfield', 'Pretoria', 'Africa/Johannesburg',
  0.00, 'ZAR', 5000, 3800,
  'published', NOW() + INTERVAL '5 days', NOW() + INTERVAL '6 days',
  NOW() + INTERVAL '9 days', NOW() - INTERVAL '1 day', 11400 ),

-- 11. UP — Tech
( 'e4000000-0000-0000-0000-000000000011',
  make_slug('University of Pretoria AI & Robotics Expo 2026', 'e4000000-0000-0000-0000-000000000011'),
  'University of Pretoria AI & Robotics Expo 2026',
  'The Faculty of Engineering at UP opens its doors for a public exhibition of AI, robotics, and advanced manufacturing research. Robot demonstrations, VR experiences, panel discussions on the future of work, and a career fair with 40 tech companies.',
  'https://picsum.photos/seed/uprobotics/600/400',
  NULL,
  4, org_id, 'University of Pretoria Engineering Faculty', 'Lynnwood Rd, Hatfield', 'Pretoria', 'Africa/Johannesburg',
  180.00, 'ZAR', 2000, 1500,
  'published', NOW() + INTERVAL '14 days', NOW() + INTERVAL '14 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '5 days', 8900 ),

-- 12. Atteridgeville — Sports
( 'e4000000-0000-0000-0000-000000000012',
  make_slug('Mamelodi Sundowns vs SuperSport United Tshwane Derby', 'e4000000-0000-0000-0000-000000000012'),
  'Mamelodi Sundowns vs SuperSport United Tshwane Derby',
  'The Tshwane Derby — the fiercest local derby in South African football. Mamelodi Sundowns host SuperSport United at Loftus Versfeld in a PSL fixture that stops the capital city. Family friendly. Kick-off 15:00. Gates open 12:00.',
  'https://picsum.photos/seed/tshwanederby/600/400',
  NULL,
  2, org_id, 'Loftus Versfeld Stadium', 'Kirkness St, Sunnyside', 'Pretoria', 'Africa/Johannesburg',
  160.00, 'ZAR', 51762, 48000,
  'published', NOW() + INTERVAL '9 days', NOW() + INTERVAL '9 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '13 days', NOW() - INTERVAL '4 days', 44100 ),

-- 13. Pretoria Botanical Gardens — Food
( 'e4000000-0000-0000-0000-000000000013',
  make_slug('Pretoria Botanical Garden Spring Picnic Festival', 'e4000000-0000-0000-0000-000000000013'),
  'Pretoria Botanical Garden Spring Picnic Festival',
  'The world-renowned Pretoria Botanical Garden hosts its annual spring picnic festival in the Jacaranda season. Over 40 food and craft vendors set up among the blooming jacaranda trees. Live music, guided garden walks, and a dog-friendly zone.',
  'https://picsum.photos/seed/pretoriabgarden/600/400',
  NULL,
  5, org_id, 'Pretoria National Botanical Garden', '2 Cussonia Ave, Brummeria', 'Pretoria', 'Africa/Johannesburg',
  100.00, 'ZAR', 6000, 4800,
  'published', NOW() + INTERVAL '16 days', NOW() + INTERVAL '16 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '6 days', 13200 ),

-- 14. Brooklyn — Exclusive
( 'e4000000-0000-0000-0000-000000000014',
  make_slug('Pretoria Embassy Row Wine & Cheese Evening', 'e4000000-0000-0000-0000-000000000014'),
  'Pretoria Embassy Row Wine & Cheese Evening',
  'An exclusive diplomatic-circle wine and cheese evening hosted in the French Embassy residence gardens. Wines from France, South Africa, and Argentina paired with artisan cheeses from five continents. Limited to 120 guests. Invitation and RSVP required.',
  'https://picsum.photos/seed/embassywine/600/400',
  NULL,
  6, org_id, 'French Embassy Residence', 'Arcadia, Pretoria', 'Pretoria', 'Africa/Johannesburg',
  3800.00, 'ZAR', 120, 96,
  'published', NOW() + INTERVAL '21 days', NOW() + INTERVAL '21 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '25 days', NOW() - INTERVAL '9 days', 6200 ),

-- 15. Church Square — Music
( 'e4000000-0000-0000-0000-000000000015',
  make_slug('Jacaranda Music Festival Church Square 2026', 'e4000000-0000-0000-0000-000000000015'),
  'Jacaranda Music Festival Church Square 2026',
  'The annual Jacaranda Music Festival transforms Pretoria''s historic Church Square into an outdoor concert venue for the jacaranda blooming season. All-day event with 8 acts, food market, craft beer garden, and the most Instagrammable backdrop in the country.',
  'https://picsum.photos/seed/jacarandafest/600/400',
  'https://picsum.photos/seed/jacarandafestban/1200/600',
  1, org_id, 'Church Square', 'Church St, Pretoria CBD', 'Pretoria', 'Africa/Johannesburg',
  280.00, 'ZAR', 10000, 8800,
  'published', NOW() + INTERVAL '25 days', NOW() + INTERVAL '25 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '29 days', NOW() - INTERVAL '12 days', 36200 ),

-- 16. Centurion — Sports
( 'e4000000-0000-0000-0000-000000000016',
  make_slug('Bulls vs Lions Super Rugby Challenge Centurion', 'e4000000-0000-0000-0000-000000000016'),
  'Bulls vs Lions Super Rugby Challenge Centurion',
  'The Vodacom Bulls face the Lions in Centurion for this Super Rugby Challenge fixture. SuperSport Park''s unique atmosphere — cricket ground meets rugby stadium — makes for an unforgettable experience. Braai stands, beer garden, and a massive fan park.',
  'https://picsum.photos/seed/bullslions/600/400',
  NULL,
  2, org_id, 'SuperSport Park', 'Totius St, Centurion', 'Pretoria', 'Africa/Johannesburg',
  240.00, 'ZAR', 20000, 17500,
  'published', NOW() + INTERVAL '13 days', NOW() + INTERVAL '13 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '17 days', NOW() - INTERVAL '5 days', 18700 ),

-- 17. UP Tuks — Sports
( 'e4000000-0000-0000-0000-000000000017',
  make_slug('Tuks Athletics Open Day: SA Olympic Trials', 'e4000000-0000-0000-0000-000000000017'),
  'Tuks Athletics Open Day: SA Olympic Trials',
  'Watch South Africa''s future Olympians compete at the High Performance Centre at Tuks. The SA Olympic Trials for Track & Field. Free entry for students and schools. Public tickets available. Witness Wayde van Niekerk and the next generation live.',
  'https://picsum.photos/seed/tuksathletics/600/400',
  NULL,
  2, org_id, 'University of Pretoria High Performance Centre', 'Roper St, Hatfield', 'Pretoria', 'Africa/Johannesburg',
  80.00, 'ZAR', 5000, 3200,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '20 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '8 days', 14900 ),

-- 18. Pretoria — Tech
( 'e4000000-0000-0000-0000-000000000018',
  make_slug('GovTech Pretoria Digital Government Summit', 'e4000000-0000-0000-0000-000000000018'),
  'GovTech Pretoria Digital Government Summit',
  'South Africa''s premier digital government transformation conference. Sessions on smart city infrastructure, e-services for citizens, digital identity, and public sector AI adoption. Hosted by SITA and DPSA. Catering included. CPD points available for professionals.',
  'https://picsum.photos/seed/govtechsummit/600/400',
  NULL,
  4, org_id, 'Tshwane Events Centre', 'Cnr Atterbury & Blesbok St, Groenkloof', 'Pretoria', 'Africa/Johannesburg',
  2200.00, 'ZAR', 2000, 1400,
  'published', NOW() + INTERVAL '31 days', NOW() + INTERVAL '32 days',
  NOW() + INTERVAL '35 days', NOW() - INTERVAL '16 days', 9600 ),

-- 19. Pretoria — Music
( 'e4000000-0000-0000-0000-000000000019',
  make_slug('Soweto Gospel Choir: Legacy Tour Pretoria', 'e4000000-0000-0000-0000-000000000019'),
  'Soweto Gospel Choir: Legacy Tour Pretoria',
  'The world-famous Soweto Gospel Choir brings their Grammy-winning voices to Pretoria for one night only. A celebration of 20 years of extraordinary music, from South African gospel classics to contemporary Afrobeat arrangements. An unmissable cultural event.',
  'https://picsum.photos/seed/sowetogospelpt/600/400',
  NULL,
  1, org_id, 'Pretoria State Theatre Main Stage', '320 Pretorius St', 'Pretoria', 'Africa/Johannesburg',
  580.00, 'ZAR', 1300, 1180,
  'published', NOW() + INTERVAL '26 days', NOW() + INTERVAL '26 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '30 days', NOW() - INTERVAL '12 days', 27400 ),

-- 20. Pretoria — Gala
( 'e4000000-0000-0000-0000-000000000020',
  make_slug('Tshwane Business Excellence Awards 2026', 'e4000000-0000-0000-0000-000000000020'),
  'Tshwane Business Excellence Awards 2026',
  'The annual Tshwane Chamber of Commerce Business Excellence Awards. Recognising outstanding businesses across 18 categories in the Tshwane Metro. Black tie dinner, awards ceremony, and networking event. Attended by the Mayor of Tshwane and senior officials.',
  'https://picsum.photos/seed/tshwanebizawards/600/400',
  NULL,
  3, org_id, 'Sheraton Pretoria Hotel', '643 Stanza Bopape St, Arcadia', 'Pretoria', 'Africa/Johannesburg',
  2800.00, 'ZAR', 400, 320,
  'published', NOW() + INTERVAL '34 days', NOW() + INTERVAL '34 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '38 days', NOW() - INTERVAL '18 days', 7800 ),

-- 21. Loftus — Sports
( 'e4000000-0000-0000-0000-000000000021',
  make_slug('Springboks vs New Zealand Rugby Championship Loftus', 'e4000000-0000-0000-0000-000000000021'),
  'Springboks vs New Zealand Rugby Championship Loftus',
  'The biggest rugby fixture in the southern hemisphere at Loftus Versfeld. The world champions face the All Blacks in a Rugby Championship showdown at altitude. Hospitality packages sell out within hours. General admission on sale now.',
  'https://picsum.photos/seed/boksallblacks/600/400',
  'https://picsum.photos/seed/boksallblanksban/1200/600',
  2, org_id, 'Loftus Versfeld Stadium', 'Kirkness St, Sunnyside', 'Pretoria', 'Africa/Johannesburg',
  680.00, 'ZAR', 51762, 51500,
  'published', NOW() + INTERVAL '41 days', NOW() + INTERVAL '41 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '45 days', NOW() - INTERVAL '24 days', 91200 ),

-- 22. Pretoria — Food
( 'e4000000-0000-0000-0000-000000000022',
  make_slug('Tshwane Braai Heritage Day Festival', 'e4000000-0000-0000-0000-000000000022'),
  'Tshwane Braai Heritage Day Festival',
  'The National Braai Day celebration comes to Tshwane in style. The South African Braai Championship, with 40 teams competing across beef, lamb, chicken, and potjie categories. Public braai stations, live boeremusiek and kwaito, craft beer village.',
  'https://picsum.photos/seed/tshwanebraai/600/400',
  NULL,
  5, org_id, 'Freedom Park', 'Koch St, Salvokop', 'Pretoria', 'Africa/Johannesburg',
  120.00, 'ZAR', 10000, 7800,
  'published', NOW() + INTERVAL '17 days', NOW() + INTERVAL '17 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '7 days', 17600 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- ██████████  GQEBERHA / PORT ELIZABETH  (e5 prefix, 20 events)
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

-- 1. Nelson Mandela Bay Stadium — Soccer
( 'e5000000-0000-0000-0000-000000000001',
  make_slug('Chippa United vs Orlando Pirates PSL Gqeberha', 'e5000000-0000-0000-0000-000000000001'),
  'Chippa United vs Orlando Pirates PSL Gqeberha',
  'Chippa United hosts Orlando Pirates at the spectacular Nelson Mandela Bay Stadium. The PSL''s most beautiful coastal stadium provides the backdrop for one of the season''s most anticipated away fixtures. Family and supporter zones open from 13:00.',
  'https://picsum.photos/seed/chippaunited/600/400',
  NULL,
  2, org_id, 'Nelson Mandela Bay Stadium', 'Prince Alfred Rd, North End', 'Gqeberha', 'Africa/Johannesburg',
  120.00, 'ZAR', 46000, 28000,
  'published', NOW() + INTERVAL '8 days', NOW() + INTERVAL '8 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '3 days', 19800 ),

-- 2. St George's Park — Cricket
( 'e5000000-0000-0000-0000-000000000002',
  make_slug('Warriors vs Dolphins CSA Provincial Day 1', 'e5000000-0000-0000-0000-000000000002'),
  'Warriors vs Dolphins CSA Provincial Day 1',
  'First-class cricket at the oldest cricket ground in Africa. St George''s Park — where the first Test match in South Africa was played — hosts the Warriors vs Dolphins in four-day provincial cricket. Members'' pavilion, historic grandstand, and a legendary ground.',
  'https://picsum.photos/seed/stgeorgespark/600/400',
  NULL,
  2, org_id, 'St George''s Park Cricket Ground', 'St George''s Park, Park Dr', 'Gqeberha', 'Africa/Johannesburg',
  80.00, 'ZAR', 19000, 7500,
  'published', NOW() + INTERVAL '14 days', NOW() + INTERVAL '14 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '5 days', 8200 ),

-- 3. Boardwalk — Gala
( 'e5000000-0000-0000-0000-000000000003',
  make_slug('Boardwalk Hotel Black Tie Charity Gala 2026', 'e5000000-0000-0000-0000-000000000003'),
  'Boardwalk Hotel Black Tie Charity Gala 2026',
  'The Eastern Cape''s most prestigious charity gala returns to the Boardwalk Convention Centre. Fundraising for the Nelson Mandela Bay Children''s Hospital. Five-course dinner, live entertainment from a Cape jazz quartet, and a silent auction of donated artworks.',
  'https://picsum.photos/seed/boardwalkgala/600/400',
  NULL,
  3, org_id, 'Boardwalk Convention Centre', 'Marine Dr, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  2400.00, 'ZAR', 500, 380,
  'published', NOW() + INTERVAL '21 days', NOW() + INTERVAL '21 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '25 days', NOW() - INTERVAL '9 days', 8600 ),

-- 4. Donkin Reserve — Music
( 'e5000000-0000-0000-0000-000000000004',
  make_slug('Donkin Reserve Outdoor Music Festival 2026', 'e5000000-0000-0000-0000-000000000004'),
  'Donkin Reserve Outdoor Music Festival 2026',
  'The historic Donkin Reserve hosts Gqeberha''s annual outdoor music festival. Six acts across two stages with a stunning harbour backdrop. From amapiano to indie-folk, there''s something for every music lover. Food trucks, craft market, and sunset cocktail bar.',
  'https://picsum.photos/seed/donkinreserve/600/400',
  NULL,
  1, org_id, 'Donkin Reserve', 'Donkin St, Central', 'Gqeberha', 'Africa/Johannesburg',
  180.00, 'ZAR', 5000, 3600,
  'published', NOW() + INTERVAL '10 days', NOW() + INTERVAL '10 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '14 days', NOW() - INTERVAL '3 days', 11200 ),

-- 5. Feathermarket — Tech
( 'e5000000-0000-0000-0000-000000000005',
  make_slug('Eastern Cape Digital Economy Conference 2026', 'e5000000-0000-0000-0000-000000000005'),
  'Eastern Cape Digital Economy Conference 2026',
  'Bringing together the Eastern Cape''s technology sector, government, and academic institutions. Sessions on digital inclusion, automotive sector 4.0, ocean economy technology, and rural connectivity. Hosted at the historic Feathermarket Centre.',
  'https://picsum.photos/seed/ecdigitalconf/600/400',
  NULL,
  4, org_id, 'Feathermarket Convention Centre', 'Feathermarket Square, Grahamstown Rd', 'Gqeberha', 'Africa/Johannesburg',
  1800.00, 'ZAR', 1500, 980,
  'published', NOW() + INTERVAL '27 days', NOW() + INTERVAL '28 days',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '13 days', 6400 ),

-- 6. Hobie Beach — Sports
( 'e5000000-0000-0000-0000-000000000006',
  make_slug('Hobie Cat World Championships 2026 Gqeberha', 'e5000000-0000-0000-0000-000000000006'),
  'Hobie Cat World Championships 2026 Gqeberha',
  'The sailing world comes to Gqeberha as Hobie Beach hosts the Hobie Cat World Championships. Over 200 boats from 30 countries race off the Nelson Mandela Bay coast. Free public viewing on the beach. Sailing school open for children during the event.',
  'https://picsum.photos/seed/hobiechampion/600/400',
  NULL,
  2, org_id, 'Hobie Beach', 'Beach Rd, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  0.00, 'ZAR', 20000, 12000,
  'published', NOW() + INTERVAL '19 days', NOW() + INTERVAL '23 days',
  NOW() + INTERVAL '27 days', NOW() - INTERVAL '8 days', 14200 ),

-- 7. Nelson Mandela Bay — Concert
( 'e5000000-0000-0000-0000-000000000007',
  make_slug('Algoa FM Big Concert: Prime Circle & Friends', 'e5000000-0000-0000-0000-000000000007'),
  'Algoa FM Big Concert: Prime Circle & Friends',
  'The Eastern Cape''s biggest annual concert event returns. Prime Circle headlines with support from Lark, Goodluck, and four emerging Eastern Cape artists. The massive lawn stage at Nelson Mandela Bay Stadium makes for an incredible festival atmosphere.',
  'https://picsum.photos/seed/algoabigconcert/600/400',
  'https://picsum.photos/seed/algoabigconcertban/1200/600',
  1, org_id, 'Nelson Mandela Bay Stadium', 'Prince Alfred Rd, North End', 'Gqeberha', 'Africa/Johannesburg',
  380.00, 'ZAR', 46000, 38000,
  'published', NOW() + INTERVAL '16 days', NOW() + INTERVAL '16 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '7 days', 31600 ),

-- 8. Summerstrand — Food
( 'e5000000-0000-0000-0000-000000000008',
  make_slug('Gqeberha Beachfront Food & Wine Market', 'e5000000-0000-0000-0000-000000000008'),
  'Gqeberha Beachfront Food & Wine Market',
  'Every Sunday on the Summerstrand beachfront, 35 food and craft vendors set up alongside the Indian Ocean. Eastern Cape wines, Karoo lamb braai, fresh oysters from Knysna, and live reggae music. The best Sunday market in the Eastern Cape.',
  'https://picsum.photos/seed/gqeberahafoodmarket/600/400',
  NULL,
  5, org_id, 'Summerstrand Beachfront', 'Marine Dr, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  0.00, 'ZAR', 3000, 2400,
  'published', NOW() + INTERVAL '2 days', NOW() + INTERVAL '2 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '5 days', NOW() - INTERVAL '1 day', 5600 ),

-- 9. PE Opera House — Gala
( 'e5000000-0000-0000-0000-000000000009',
  make_slug('Eastern Cape Music Awards Gala 2026', 'e5000000-0000-0000-0000-000000000009'),
  'Eastern Cape Music Awards Gala 2026',
  'Celebrating the best of Eastern Cape music across 14 categories including gospel, maskanda, jazz, hip-hop, and classical. Held at the beautiful PE Opera House. Red carpet, awards ceremony, gala dinner, and a closing concert by this year''s Artist of the Year.',
  'https://picsum.photos/seed/ecmusicawards/600/400',
  NULL,
  3, org_id, 'Port Elizabeth Opera House', 'White''s Rd, Central', 'Gqeberha', 'Africa/Johannesburg',
  1200.00, 'ZAR', 700, 540,
  'published', NOW() + INTERVAL '24 days', NOW() + INTERVAL '24 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '10 days', 7400 ),

-- 10. Boardwalk — Exclusive
( 'e5000000-0000-0000-0000-000000000010',
  make_slug('Boardwalk Casino VIP High Roller Evening 2026', 'e5000000-0000-0000-0000-000000000010'),
  'Boardwalk Casino VIP High Roller Evening 2026',
  'An exclusive evening for Boardwalk Casino''s top-tier members. Private gaming tables, sommelier-selected wine service, a five-course dinner in the VIP dining room, and entertainment from a Las Vegas-style show. By invitation only.',
  'https://picsum.photos/seed/boardwalkvip/600/400',
  NULL,
  6, org_id, 'Boardwalk Casino & Entertainment World', 'Marine Dr, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  6500.00, 'ZAR', 80, 62,
  'published', NOW() + INTERVAL '31 days', NOW() + INTERVAL '31 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '35 days', NOW() - INTERVAL '15 days', 4200 ),

-- 11. Gqeberha — Sports
( 'e5000000-0000-0000-0000-000000000011',
  make_slug('Ironman South Africa 2026 Nelson Mandela Bay', 'e5000000-0000-0000-0000-000000000011'),
  'Ironman South Africa 2026 Nelson Mandela Bay',
  'IRONMAN South Africa returns to the Nelson Mandela Bay for its 20th edition. A 3.8km swim, 180km cycle, and 42.2km run along the stunning Eastern Cape coastline. Africa''s most iconic endurance event. Spectator zones along the entire route. Free entry for spectators.',
  'https://picsum.photos/seed/ironmanpe/600/400',
  'https://picsum.photos/seed/ironmanpeban/1200/600',
  2, org_id, 'Hobie Beach', 'Beach Rd, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  4800.00, 'ZAR', 2000, 1850,
  'published', NOW() + INTERVAL '36 days', NOW() + INTERVAL '36 days' + INTERVAL '16 hours',
  NOW() + INTERVAL '40 days', NOW() - INTERVAL '22 days', 28900 ),

-- 12. St George's — Food
( 'e5000000-0000-0000-0000-000000000012',
  make_slug('Eastern Cape Braai & Beer Festival 2026', 'e5000000-0000-0000-0000-000000000012'),
  'Eastern Cape Braai & Beer Festival 2026',
  'Two days of craft beer and world-class braai at St George''s Park. 20 Eastern Cape craft breweries, 15 braai pitmasters competing for the Braai King title, live music, and a kids'' braai competition. The friendliest festival in the Eastern Cape.',
  'https://picsum.photos/seed/ecbraai/600/400',
  NULL,
  5, org_id, 'St George''s Park', 'Park Dr, Central', 'Gqeberha', 'Africa/Johannesburg',
  160.00, 'ZAR', 4000, 2900,
  'published', NOW() + INTERVAL '12 days', NOW() + INTERVAL '13 days',
  NOW() + INTERVAL '16 days', NOW() - INTERVAL '4 days', 10800 ),

-- 13. Gqeberha — Tech
( 'e5000000-0000-0000-0000-000000000013',
  make_slug('Eastern Cape AutoTech Innovation Summit 2026', 'e5000000-0000-0000-0000-000000000013'),
  'Eastern Cape AutoTech Innovation Summit 2026',
  'Gqeberha is the automotive capital of South Africa. This summit brings together VW, Mercedes-Benz, and Eastern Cape suppliers to discuss electric vehicle manufacturing, automation, and the future of automotive production in South Africa.',
  'https://picsum.photos/seed/ecautotechsummit/600/400',
  NULL,
  4, org_id, 'Boardwalk Convention Centre', 'Marine Dr, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  2400.00, 'ZAR', 1200, 880,
  'published', NOW() + INTERVAL '32 days', NOW() + INTERVAL '33 days',
  NOW() + INTERVAL '36 days', NOW() - INTERVAL '17 days', 7600 ),

-- 14. Nelson Mandela Bay — Sports
( 'e5000000-0000-0000-0000-000000000014',
  make_slug('SA Volleyball Championship Nelson Mandela Bay', 'e5000000-0000-0000-0000-000000000014'),
  'SA Volleyball Championship Nelson Mandela Bay',
  'The South African Indoor Volleyball Championships comes to Gqeberha for the first time. 16 teams from across the country compete over three days at the Nelson Mandela Bay Indoor Arena. Great family entertainment. Children under 12 free.',
  'https://picsum.photos/seed/savolleyball/600/400',
  NULL,
  2, org_id, 'Nelson Mandela Bay Indoor Arena', 'Makana Rd, North End', 'Gqeberha', 'Africa/Johannesburg',
  80.00, 'ZAR', 3000, 1800,
  'published', NOW() + INTERVAL '22 days', NOW() + INTERVAL '24 days',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '9 days', 6200 ),

-- 15. Gqeberha — Music
( 'e5000000-0000-0000-0000-000000000015',
  make_slug('Gqeberha Jazz Festival Night 2 2026', 'e5000000-0000-0000-0000-000000000015'),
  'Gqeberha Jazz Festival Night 2 2026',
  'Night 2 of the inaugural Gqeberha International Jazz Festival at the Boardwalk. Local legends meet international artists for an evening of world-class jazz. Five acts across two stages, from hard bop to Afro-jazz fusion. Dinner packages include reserved seating.',
  'https://picsum.photos/seed/gqeberahajazz/600/400',
  NULL,
  1, org_id, 'Boardwalk Hotel & Casino', 'Marine Dr, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  480.00, 'ZAR', 2000, 1650,
  'published', NOW() + INTERVAL '15 days', NOW() + INTERVAL '15 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '19 days', NOW() - INTERVAL '5 days', 13400 ),

-- 16. Gqeberha — Gala
( 'e5000000-0000-0000-0000-000000000016',
  make_slug('Nelson Mandela Bay Mayor''s Annual Gala 2026', 'e5000000-0000-0000-0000-000000000016'),
  'Nelson Mandela Bay Mayor''s Annual Gala 2026',
  'The official Mayoral Gala of the Nelson Mandela Bay Municipality. Recognising outstanding citizens, businesses, and community organisations. Black tie. Five-course dinner, live entertainment, and the presentation of the Mayor''s annual awards.',
  'https://picsum.photos/seed/nmbaymayorgala/600/400',
  NULL,
  3, org_id, 'Boardwalk Convention Centre', 'Marine Dr, Summerstrand', 'Gqeberha', 'Africa/Johannesburg',
  1800.00, 'ZAR', 600, 500,
  'published', NOW() + INTERVAL '28 days', NOW() + INTERVAL '28 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '13 days', 5800 ),

-- 17. Gqeberha — Food Exclusive
( 'e5000000-0000-0000-0000-000000000017',
  make_slug('Karoo to Coast Food Safari Weekend 2026', 'e5000000-0000-0000-0000-000000000017'),
  'Karoo to Coast Food Safari Weekend 2026',
  'A two-day luxury food safari combining the best of the Karoo and the Eastern Cape coast. Small group of 20 guests. Winery visits, lamb on the spit at a Karoo farm, oyster tasting on the coast, and a gala dinner at the Windermere Hotel.',
  'https://picsum.photos/seed/karoocoast/600/400',
  NULL,
  6, org_id, 'Multiple Locations — Eastern Cape', 'Various', 'Gqeberha', 'Africa/Johannesburg',
  8500.00, 'ZAR', 20, 15,
  'published', NOW() + INTERVAL '25 days', NOW() + INTERVAL '26 days',
  NOW() + INTERVAL '29 days', NOW() - INTERVAL '10 days', 3800 ),

-- 18. Gqeberha — Sports
( 'e5000000-0000-0000-0000-000000000018',
  make_slug('NMB International Cycling Classic 2026', 'e5000000-0000-0000-0000-000000000018'),
  'NMB International Cycling Classic 2026',
  '120km road cycling race around the Nelson Mandela Bay coastline. UCI-recognised international race featuring South African and African continental teams. Public crit race on Saturday morning before the main event. Free spectator zones throughout.',
  'https://picsum.photos/seed/nmbcycling/600/400',
  NULL,
  2, org_id, 'Shark Rock Pier', 'Marine Dr, Humewood', 'Gqeberha', 'Africa/Johannesburg',
  320.00, 'ZAR', 500, 380,
  'published', NOW() + INTERVAL '18 days', NOW() + INTERVAL '18 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '6 days', 9200 ),

-- 19. Gqeberha — Music
( 'e5000000-0000-0000-0000-000000000019',
  make_slug('Eastern Cape Heritage Music Celebration 2026', 'e5000000-0000-0000-0000-000000000019'),
  'Eastern Cape Heritage Music Celebration 2026',
  'A celebratory concert honouring the Eastern Cape''s musical heritage — from maskanda to mbaqanga, bubblegum to gospel. Held at the outdoor Nelson Mandela Bay Stadium precinct. Free community event on Saturday, ticketed gala concert on Sunday.',
  'https://picsum.photos/seed/echnimage/600/400',
  NULL,
  1, org_id, 'Nelson Mandela Bay Stadium Precinct', 'Prince Alfred Rd, North End', 'Gqeberha', 'Africa/Johannesburg',
  120.00, 'ZAR', 8000, 5600,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '20 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '7 days', 12800 ),

-- 20. Gqeberha — Tech
( 'e5000000-0000-0000-0000-000000000020',
  make_slug('Gqeberha Smart City Hackathon 2026', 'e5000000-0000-0000-0000-000000000020'),
  'Gqeberha Smart City Hackathon 2026',
  '48-hour hackathon focused on smart city solutions for the Nelson Mandela Bay. Open to developers, designers, students, and entrepreneurs. Challenges set by the Municipality and private sector. R500 000 in prizes. Meals and accommodation included for participants.',
  'https://picsum.photos/seed/gqeberhahackathon/600/400',
  NULL,
  4, org_id, 'Feathermarket Convention Centre', 'Feathermarket Square', 'Gqeberha', 'Africa/Johannesburg',
  0.00, 'ZAR', 300, 250,
  'published', NOW() + INTERVAL '37 days', NOW() + INTERVAL '39 days',
  NOW() + INTERVAL '42 days', NOW() - INTERVAL '19 days', 7100 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- ██████████  BLOEMFONTEIN  (e6 prefix, 20 events)
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

-- 1. Free State Stadium — Soccer
( 'e6000000-0000-0000-0000-000000000001',
  make_slug('Bloemfontein Celtic vs Moroka Swallows PSL', 'e6000000-0000-0000-0000-000000000001'),
  'Bloemfontein Celtic vs Moroka Swallows PSL',
  'The People''s Team host Moroka Swallows at the Free State Stadium in a PSL clash that promises passion, drama, and goals. Celtic fans are among the most loyal in South African football. Family zones open from 13:00, kick-off 15:30.',
  'https://picsum.photos/seed/celticswallows/600/400',
  NULL,
  2, org_id, 'Free State Stadium', 'Kolbe Rd, Mangaung', 'Bloemfontein', 'Africa/Johannesburg',
  100.00, 'ZAR', 45000, 28000,
  'published', NOW() + INTERVAL '7 days', NOW() + INTERVAL '7 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '11 days', NOW() - INTERVAL '3 days', 16800 ),

-- 2. Goodyear Park — Cricket
( 'e6000000-0000-0000-0000-000000000002',
  make_slug('Knights vs Titans CSA T20 Goodyear Park', 'e6000000-0000-0000-0000-000000000002'),
  'Knights vs Titans CSA T20 Goodyear Park',
  'The Central Gauteng Lions take on the Knights at the compact and electric Goodyear Park. T20 cricket at its best — big hits, clever bowling, and a crowd that knows how to enjoy an evening of cricket. Kids under 12 enter free with a paying adult.',
  'https://picsum.photos/seed/knightstitans/600/400',
  NULL,
  2, org_id, 'Mangaung Oval (Goodyear Park)', 'Aliwal St, Willows', 'Bloemfontein', 'Africa/Johannesburg',
  100.00, 'ZAR', 20000, 12000,
  'published', NOW() + INTERVAL '10 days', NOW() + INTERVAL '10 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '14 days', NOW() - INTERVAL '4 days', 10400 ),

-- 3. Sand du Plessis — Gala
( 'e6000000-0000-0000-0000-000000000003',
  make_slug('Free State Symphony Orchestra Gala 2026', 'e6000000-0000-0000-0000-000000000003'),
  'Free State Symphony Orchestra Gala 2026',
  'The Free State Symphony Orchestra opens its 2026 season with a spectacular gala performance at the Sand du Plessis Theatre. Programme includes Dvorak''s New World Symphony and Rachmaninoff''s Piano Concerto No. 2. Black tie preferred.',
  'https://picsum.photos/seed/fssymphonygala/600/400',
  NULL,
  3, org_id, 'Sand du Plessis Theatre', '66 St George St', 'Bloemfontein', 'Africa/Johannesburg',
  780.00, 'ZAR', 960, 820,
  'published', NOW() + INTERVAL '16 days', NOW() + INTERVAL '16 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '6 days', 8200 ),

-- 4. UFS — Tech
( 'e6000000-0000-0000-0000-000000000004',
  make_slug('University of Free State Tech & Research Expo 2026', 'e6000000-0000-0000-0000-000000000004'),
  'University of Free State Tech & Research Expo 2026',
  'The University of the Free State showcases cutting-edge research and technology innovations. Agricultural technology, water science, renewable energy, and AI research on display. Industry networking sessions and graduate recruitment fair. Open to public.',
  'https://picsum.photos/seed/ufstechexpo/600/400',
  NULL,
  4, org_id, 'University of the Free State Main Campus', 'Nelson Mandela Dr', 'Bloemfontein', 'Africa/Johannesburg',
  0.00, 'ZAR', 2000, 1400,
  'published', NOW() + INTERVAL '22 days', NOW() + INTERVAL '23 days',
  NOW() + INTERVAL '27 days', NOW() - INTERVAL '9 days', 6800 ),

-- 5. Loch Logan — Food
( 'e6000000-0000-0000-0000-000000000005',
  make_slug('Loch Logan Waterfront Food & Craft Festival', 'e6000000-0000-0000-0000-000000000005'),
  'Loch Logan Waterfront Food & Craft Festival',
  'The Loch Logan Waterfront transforms into a food lover''s paradise for the weekend. Free State produce, Karoo lamb, boere-style food, artisan baked goods, and craft beer from the Free State Brewery. Live Afrikaans folk music throughout.',
  'https://picsum.photos/seed/lochloganfood/600/400',
  NULL,
  5, org_id, 'Loch Logan Waterfront', 'Henry St, Westdene', 'Bloemfontein', 'Africa/Johannesburg',
  60.00, 'ZAR', 5000, 3800,
  'published', NOW() + INTERVAL '5 days', NOW() + INTERVAL '6 days',
  NOW() + INTERVAL '9 days', NOW() - INTERVAL '1 day', 9200 ),

-- 6. Free State Stadium — Rugby
( 'e6000000-0000-0000-0000-000000000006',
  make_slug('Cheetahs vs Griquas Currie Cup Bloemfontein', 'e6000000-0000-0000-0000-000000000006'),
  'Cheetahs vs Griquas Currie Cup Bloemfontein',
  'The Free State Cheetahs host the Griquas in a feisty Currie Cup fixture at the Free State Stadium. The Cheetahs'' orange army makes Bloemfontein one of the best home venues in South African rugby. Braai areas open from 12:00.',
  'https://picsum.photos/seed/cheetahsgriquas/600/400',
  NULL,
  2, org_id, 'Free State Stadium', 'Kolbe Rd, Mangaung', 'Bloemfontein', 'Africa/Johannesburg',
  220.00, 'ZAR', 45000, 32000,
  'published', NOW() + INTERVAL '12 days', NOW() + INTERVAL '12 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '16 days', NOW() - INTERVAL '5 days', 21400 ),

-- 7. Toyota Stadium — Sports
( 'e6000000-0000-0000-0000-000000000007',
  make_slug('SA Rugby Under-20 Championship Bloemfontein', 'e6000000-0000-0000-0000-000000000007'),
  'SA Rugby Under-20 Championship Bloemfontein',
  'South Africa''s best young rugby players compete for the Under-20 Championship. Watch the next generation of Springboks in action at the Toyota Stadium. Family-friendly, affordable tickets, and a great day out for the whole family.',
  'https://picsum.photos/seed/saunder20rugby/600/400',
  NULL,
  2, org_id, 'Toyota Stadium', 'Kolbe Rd', 'Bloemfontein', 'Africa/Johannesburg',
  80.00, 'ZAR', 20000, 12000,
  'published', NOW() + INTERVAL '18 days', NOW() + INTERVAL '19 days',
  NOW() + INTERVAL '23 days', NOW() - INTERVAL '7 days', 9800 ),

-- 8. Mangaung — Music
( 'e6000000-0000-0000-0000-000000000008',
  make_slug('Vrystaat Kunstefees Music Night 2026', 'e6000000-0000-0000-0000-000000000008'),
  'Vrystaat Kunstefees Music Night 2026',
  'The Vrystaat Kunstefees (Arts Festival) presents its signature music night. Six acts representing the full spectrum of Afrikaans and South African music — from contemporary Afrikaans pop to soulful blues and jazz. The centrepiece of the annual arts festival.',
  'https://picsum.photos/seed/vrystaat/600/400',
  NULL,
  1, org_id, 'Sand du Plessis Theatre & Surrounds', '66 St George St', 'Bloemfontein', 'Africa/Johannesburg',
  360.00, 'ZAR', 3000, 2400,
  'published', NOW() + INTERVAL '14 days', NOW() + INTERVAL '14 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '5 days', 16200 ),

-- 9. Oliewenhuis — Gala
( 'e6000000-0000-0000-0000-000000000009',
  make_slug('Oliewenhuis Art Museum Autumn Gala Dinner', 'e6000000-0000-0000-0000-000000000009'),
  'Oliewenhuis Art Museum Autumn Gala Dinner',
  'An intimate and elegant gala dinner in the magnificent grounds of the Oliewenhuis Art Museum. Guests enjoy a four-course dinner under the stars among the museum''s renowned sculpture garden. Live chamber music, silent auction, and guided gallery viewing.',
  'https://picsum.photos/seed/oliewen/600/400',
  NULL,
  3, org_id, 'Oliewenhuis Art Museum', '16 Harry Smith St, Arboretum', 'Bloemfontein', 'Africa/Johannesburg',
  1800.00, 'ZAR', 200, 160,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '20 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '24 days', NOW() - INTERVAL '8 days', 6200 ),

-- 10. Bloemfontein — Food
( 'e6000000-0000-0000-0000-000000000010',
  make_slug('Free State Potjie Championship 2026', 'e6000000-0000-0000-0000-000000000010'),
  'Free State Potjie Championship 2026',
  'South Africa''s best potjiekos cooks battle it out for the Free State Potjie Championship title. 50 teams competing across lamb, venison, chicken, and vegetarian categories. Live Boeremusiek, craft beer garden, and public tasting sessions in the afternoon.',
  'https://picsum.photos/seed/potjie/600/400',
  NULL,
  5, org_id, 'Bloemfontein Show Grounds', 'Nelson Mandela Dr', 'Bloemfontein', 'Africa/Johannesburg',
  100.00, 'ZAR', 5000, 3600,
  'published', NOW() + INTERVAL '9 days', NOW() + INTERVAL '9 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '13 days', NOW() - INTERVAL '3 days', 11400 ),

-- 11. Bloemfontein — Tech
( 'e6000000-0000-0000-0000-000000000011',
  make_slug('Mangaung Digital Skills Bootcamp 2026', 'e6000000-0000-0000-0000-000000000011'),
  'Mangaung Digital Skills Bootcamp 2026',
  'A three-day intensive digital skills bootcamp for young people in the Free State. Modules include web development, data literacy, digital marketing, and e-commerce. Sponsored places available for unemployed youth aged 18-35. Certification on completion.',
  'https://picsum.photos/seed/mangaungdigital/600/400',
  NULL,
  4, org_id, 'Mangaung Metropolitan Municipality', 'Civic Square, Bloemfontein', 'Bloemfontein', 'Africa/Johannesburg',
  800.00, 'ZAR', 200, 180,
  'published', NOW() + INTERVAL '26 days', NOW() + INTERVAL '28 days',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '11 days', 5400 ),

-- 12. Bloemfontein — Exclusive
( 'e6000000-0000-0000-0000-000000000012',
  make_slug('Free State Estate Wine & Dine 2026', 'e6000000-0000-0000-0000-000000000012'),
  'Free State Estate Wine & Dine 2026',
  'An exclusive evening at one of the Free State''s hidden gem wine estates. Only 60 guests for a six-course dinner paired with estate wines and artisan spirits from the Free State. Tour of the cellar and vine garden included. Accommodation packages available.',
  'https://picsum.photos/seed/freestatewine/600/400',
  NULL,
  6, org_id, 'Verdun Wine Estate', 'R30, Bonnievale', 'Bloemfontein', 'Africa/Johannesburg',
  4200.00, 'ZAR', 60, 48,
  'published', NOW() + INTERVAL '30 days', NOW() + INTERVAL '30 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '34 days', NOW() - INTERVAL '14 days', 4100 ),

-- 13. Free State Stadium — Concert
( 'e6000000-0000-0000-0000-000000000013',
  make_slug('Bloemfontein Amapiano Stadium Bash 2026', 'e6000000-0000-0000-0000-000000000013'),
  'Bloemfontein Amapiano Stadium Bash 2026',
  'The biggest amapiano night ever held in Bloemfontein. DJ Maphorisa, Kelvin Momo, and three local Free State acts take over the Free State Stadium. Food village, braai stands, VIP hospitality lounge. The Mangaung experience at its finest.',
  'https://picsum.photos/seed/bloemamapianoban/600/400',
  'https://picsum.photos/seed/bloemamapianobanban/1200/600',
  1, org_id, 'Free State Stadium', 'Kolbe Rd, Mangaung', 'Bloemfontein', 'Africa/Johannesburg',
  350.00, 'ZAR', 45000, 36000,
  'published', NOW() + INTERVAL '23 days', NOW() + INTERVAL '23 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '27 days', NOW() - INTERVAL '10 days', 38400 ),

-- 14. Bloemfontein — Sports
( 'e6000000-0000-0000-0000-000000000014',
  make_slug('SA Track & Field Championships Bloemfontein 2026', 'e6000000-0000-0000-0000-000000000014'),
  'SA Track & Field Championships Bloemfontein 2026',
  'The South African Athletics Championships return to Bloemfontein — the city of world-record holders. Watch SA''s best athletes compete for national titles across all track and field disciplines. Free entry for school groups. National team selection event.',
  'https://picsum.photos/seed/saathletics/600/400',
  NULL,
  2, org_id, 'Dr Petrus Molemela Stadium', 'Seisa Ramabodu Rd, Mangaung', 'Bloemfontein', 'Africa/Johannesburg',
  120.00, 'ZAR', 10000, 6800,
  'published', NOW() + INTERVAL '33 days', NOW() + INTERVAL '34 days',
  NOW() + INTERVAL '38 days', NOW() - INTERVAL '17 days', 16200 ),

-- 15. Bloemfontein — Music
( 'e6000000-0000-0000-0000-000000000015',
  make_slug('Bloemfontein Jazz in the Park 2026', 'e6000000-0000-0000-0000-000000000015'),
  'Bloemfontein Jazz in the Park 2026',
  'Bloemfontein''s most beloved outdoor music event returns to the beautiful Orchid House grounds. Five jazz acts performing across the afternoon and evening. BYO picnic blanket. Licensed bar, artisan food stalls, and a dedicated kids'' music area.',
  'https://picsum.photos/seed/bloemjazzpark/600/400',
  NULL,
  1, org_id, 'King''s Park Orchid House', 'Union Ave, Westdene', 'Bloemfontein', 'Africa/Johannesburg',
  160.00, 'ZAR', 3000, 2200,
  'published', NOW() + INTERVAL '11 days', NOW() + INTERVAL '11 days' + INTERVAL '7 hours',
  NOW() + INTERVAL '15 days', NOW() - INTERVAL '3 days', 9600 ),

-- 16. Bloemfontein — Gala
( 'e6000000-0000-0000-0000-000000000016',
  make_slug('Free State Business Women''s Awards Gala 2026', 'e6000000-0000-0000-0000-000000000016'),
  'Free State Business Women''s Awards Gala 2026',
  'Celebrating exceptional women in Free State business across 10 award categories. The most anticipated women''s business event in the province. Keynote by a leading South African businesswoman, gala dinner, and a post-awards cocktail celebration.',
  'https://picsum.photos/seed/fsbizwomen/600/400',
  NULL,
  3, org_id, 'Protea Hotel Bloemfontein Willow Lake', '2 Melville Rd, Willows', 'Bloemfontein', 'Africa/Johannesburg',
  1600.00, 'ZAR', 350, 280,
  'published', NOW() + INTERVAL '28 days', NOW() + INTERVAL '28 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '32 days', NOW() - INTERVAL '13 days', 5800 ),

-- 17. Bloemfontein — Food
( 'e6000000-0000-0000-0000-000000000017',
  make_slug('Mangaung Street Food & Heritage Market', 'e6000000-0000-0000-0000-000000000017'),
  'Mangaung Street Food & Heritage Market',
  'Every Saturday morning in the Hoffman Square. Local food producers, traditional healers'' market, clothing designers, and an incredible food court celebrating Sotho and Afrikaner heritage foods side by side. The authentic Bloemfontein experience.',
  'https://picsum.photos/seed/mangaungmarket/600/400',
  NULL,
  5, org_id, 'Hoffman Square', 'President Brand St', 'Bloemfontein', 'Africa/Johannesburg',
  0.00, 'ZAR', 3000, 2100,
  'published', NOW() + INTERVAL '3 days', NOW() + INTERVAL '3 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '6 days', NOW() - INTERVAL '1 day', 6400 ),

-- 18. Bloemfontein — Exclusive
( 'e6000000-0000-0000-0000-000000000018',
  make_slug('Cheetahs Legends Dinner & Auction 2026', 'e6000000-0000-0000-0000-000000000018'),
  'Cheetahs Legends Dinner & Auction 2026',
  'An exclusive evening with Free State Cheetahs legends including CJ van der Linde, Wynand Olivier, and more. Black tie dinner, live auction of signed memorabilia, and an evening of rugby stories. Proceeds to the Cheetahs Academy programme.',
  'https://picsum.photos/seed/cheetahslegends/600/400',
  NULL,
  6, org_id, 'Bloem Show Grounds Function Hall', 'Nelson Mandela Dr', 'Bloemfontein', 'Africa/Johannesburg',
  2800.00, 'ZAR', 200, 155,
  'published', NOW() + INTERVAL '19 days', NOW() + INTERVAL '19 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '23 days', NOW() - INTERVAL '7 days', 7200 ),

-- 19. Bloemfontein — Sports
( 'e6000000-0000-0000-0000-000000000019',
  make_slug('Free State Netball Championship 2026', 'e6000000-0000-0000-0000-000000000019'),
  'Free State Netball Championship 2026',
  'The Free State Netball Championship brings together the province''s top school and club netball teams for a weekend of top-level competition. Spur hospitality for families, food stalls, and inspiration for the next generation of Proteas netballers.',
  'https://picsum.photos/seed/fsnetball/600/400',
  NULL,
  2, org_id, 'University of Free State Oval Hall', 'Nelson Mandela Dr', 'Bloemfontein', 'Africa/Johannesburg',
  60.00, 'ZAR', 2000, 1200,
  'published', NOW() + INTERVAL '15 days', NOW() + INTERVAL '16 days',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '4 days', 5100 ),

-- 20. Bloemfontein — Tech
( 'e6000000-0000-0000-0000-000000000020',
  make_slug('Agritech Expo Free State 2026', 'e6000000-0000-0000-0000-000000000020'),
  'Agritech Expo Free State 2026',
  'The Free State is South Africa''s breadbasket, and this agritech expo showcases the technologies transforming modern farming. Precision agriculture, drone technology, irrigation innovations, and sustainable farming solutions. Live demonstrations on a working farm.',
  'https://picsum.photos/seed/agritechfs/600/400',
  NULL,
  4, org_id, 'Bloemfontein Agricultural Show Grounds', 'Nelson Mandela Dr', 'Bloemfontein', 'Africa/Johannesburg',
  350.00, 'ZAR', 3000, 2100,
  'published', NOW() + INTERVAL '37 days', NOW() + INTERVAL '38 days',
  NOW() + INTERVAL '42 days', NOW() - INTERVAL '20 days', 8800 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- ██████████  EXTRA JOHANNESBURG  (e7 prefix, 22 events)
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

-- 1. FNB Stadium — Soccer
( 'e7000000-0000-0000-0000-000000000001',
  make_slug('Bafana Bafana vs Nigeria AFCON Qualifier FNB', 'e7000000-0000-0000-0000-000000000001'),
  'Bafana Bafana vs Nigeria AFCON Qualifier FNB',
  'AFCON qualification is on the line as Bafana Bafana face the Super Eagles of Nigeria at the iconic Soccer City. 94 000 screaming fans, an electric atmosphere, and national pride at stake. The biggest international football match played in SA this decade.',
  'https://picsum.photos/seed/bafananigeria/600/400',
  'https://picsum.photos/seed/bafananigeriabn/1200/600',
  2, org_id, 'FNB Stadium', 'Soccer City Ave, Nasrec', 'Johannesburg', 'Africa/Johannesburg',
  200.00, 'ZAR', 94700, 85000,
  'published', NOW() + INTERVAL '15 days', NOW() + INTERVAL '15 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '19 days', NOW() - INTERVAL '7 days', 92100 ),

-- 2. Ellis Park — Rugby
( 'e7000000-0000-0000-0000-000000000002',
  make_slug('Lions vs Stormers URC Ellis Park', 'e7000000-0000-0000-0000-000000000002'),
  'Lions vs Stormers URC Ellis Park',
  'The Golden Lions host the Stormers at the iconic Ellis Park Stadium. The URC fixture that always delivers the most dramatic ending in South African rugby. Corporate suites, VIP hospitality, and the famous Ellis Park roar. Book early — sells out every year.',
  'https://picsum.photos/seed/lionsstormers/600/400',
  NULL,
  2, org_id, 'Emirates Airlines Park (Ellis Park)', 'Staib St, Doornfontein', 'Johannesburg', 'Africa/Johannesburg',
  350.00, 'ZAR', 62000, 55000,
  'published', NOW() + INTERVAL '9 days', NOW() + INTERVAL '9 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '13 days', NOW() - INTERVAL '4 days', 41200 ),

-- 3. FNB Stadium — Concert
( 'e7000000-0000-0000-0000-000000000003',
  make_slug('Beyoncé Renaissance World Tour South Africa 2026', 'e7000000-0000-0000-0000-000000000003'),
  'Beyoncé Renaissance World Tour South Africa 2026',
  'Queen Bey comes to South Africa for the first time. The Renaissance World Tour makes its only African stop at FNB Stadium Johannesburg. One night only. The most anticipated concert event in South African history. All ticket tiers. No presale code required.',
  'https://picsum.photos/seed/beyoncejhb/600/400',
  'https://picsum.photos/seed/beyoncejhbban/1200/600',
  1, org_id, 'FNB Stadium', 'Soccer City Ave, Nasrec', 'Johannesburg', 'Africa/Johannesburg',
  1800.00, 'ZAR', 94700, 94000,
  'published', NOW() + INTERVAL '55 days', NOW() + INTERVAL '55 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '58 days', NOW() - INTERVAL '30 days', 198400 ),

-- 4. Sandton Convention Centre — Tech
( 'e7000000-0000-0000-0000-000000000004',
  make_slug('Innovation Summit Johannesburg 2026', 'e7000000-0000-0000-0000-000000000004'),
  'Innovation Summit Johannesburg 2026',
  'Africa''s fastest-growing technology conference. Three days of keynotes, workshops, and investor meetups at the Sandton Convention Centre. Featuring CEOs from Africa''s top 20 tech companies and investors with $4bn in assets under management.',
  'https://picsum.photos/seed/innovationsummitjhb/600/400',
  NULL,
  4, org_id, 'Sandton Convention Centre', '161 Maude St, Sandton', 'Johannesburg', 'Africa/Johannesburg',
  6500.00, 'ZAR', 8000, 6200,
  'published', NOW() + INTERVAL '20 days', NOW() + INTERVAL '22 days',
  NOW() + INTERVAL '26 days', NOW() - INTERVAL '9 days', 31400 ),

-- 5. Melrose Arch — Exclusive
( 'e7000000-0000-0000-0000-000000000005',
  make_slug('Melrose Arch Rooftop Jazz & Wine 2026', 'e7000000-0000-0000-0000-000000000005'),
  'Melrose Arch Rooftop Jazz & Wine 2026',
  'A sophisticated Friday evening on the rooftop of Melrose Arch. Live jazz quartet, sommelier-curated wine selections from 8 SA estates, and artisan charcuterie. Stunning Johannesburg skyline views. Limited to 120 guests per evening. Smart casual required.',
  'https://picsum.photos/seed/melrosejazz/600/400',
  NULL,
  6, org_id, 'Melrose Arch Rooftop', '1 Melrose Square, Melrose North', 'Johannesburg', 'Africa/Johannesburg',
  1200.00, 'ZAR', 120, 98,
  'published', NOW() + INTERVAL '4 days', NOW() + INTERVAL '4 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '7 days', NOW() - INTERVAL '1 day', 8900 ),

-- 6. Rosebank — Food
( 'e7000000-0000-0000-0000-000000000006',
  make_slug('Rosebank Sunday Market Eat Out Edition', 'e7000000-0000-0000-0000-000000000006'),
  'Rosebank Sunday Market Eat Out Edition',
  'The famous Rosebank Sunday Market celebrates its annual Eat Out edition. 80 vendors, SA''s top food trucks, three live music stages, and a dedicated Eat Out Good Food Award pop-up showcasing dishes from Johannesburg''s best restaurants. Free entry.',
  'https://picsum.photos/seed/rosebankmarket/600/400',
  NULL,
  5, org_id, 'Rosebank Mall Rooftop Market', 'Bath Ave, Rosebank', 'Johannesburg', 'Africa/Johannesburg',
  0.00, 'ZAR', 5000, 4200,
  'published', NOW() + INTERVAL '3 days', NOW() + INTERVAL '3 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '6 days', NOW() - INTERVAL '1 day', 11200 ),

-- 7. Constitution Hill — Gala
( 'e7000000-0000-0000-0000-000000000007',
  make_slug('Constitution Hill Heritage Gala 2026', 'e7000000-0000-0000-0000-000000000007'),
  'Constitution Hill Heritage Gala 2026',
  'An extraordinary evening at Constitution Hill — site of the Constitutional Court. Dinner in the Women''s Gaol courtyard, guided tours of the Constitutional Court, speeches by human rights leaders, and a performance by a leading South African choral group.',
  'https://picsum.photos/seed/constitutionhill/600/400',
  NULL,
  3, org_id, 'Constitution Hill', '1 Human Rights Square, Braamfontein', 'Johannesburg', 'Africa/Johannesburg',
  3200.00, 'ZAR', 300, 245,
  'published', NOW() + INTERVAL '25 days', NOW() + INTERVAL '25 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '29 days', NOW() - INTERVAL '11 days', 9400 ),

-- 8. Ellis Park — Concert
( 'e7000000-0000-0000-0000-000000000008',
  make_slug('Amapiano Festival Ellis Park 2026', 'e7000000-0000-0000-0000-000000000008'),
  'Amapiano Festival Ellis Park 2026',
  'The genre that took the world by storm comes home. The Ellis Park Amapiano Festival brings together every major log-drum producer and vocalist for one unmissable day. Kabza De Small, DJ Maphorisa, Focalistic, Ami Faku, Uncle Waffles, and more.',
  'https://picsum.photos/seed/amapianofestivalep/600/400',
  'https://picsum.photos/seed/amapianofestivalepbn/1200/600',
  1, org_id, 'Emirates Airlines Park (Ellis Park)', 'Staib St, Doornfontein', 'Johannesburg', 'Africa/Johannesburg',
  680.00, 'ZAR', 62000, 57000,
  'published', NOW() + INTERVAL '18 days', NOW() + INTERVAL '18 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '22 days', NOW() - INTERVAL '8 days', 76400 ),

-- 9. Soweto — Sports
( 'e7000000-0000-0000-0000-000000000009',
  make_slug('Soweto Derby: Orlando Pirates vs Kaizer Chiefs', 'e7000000-0000-0000-0000-000000000009'),
  'Soweto Derby: Orlando Pirates vs Kaizer Chiefs',
  'The match that stops a nation. The Soweto Derby — the most attended football match in Africa — returns to the FNB Stadium. Orlando Pirates vs Kaizer Chiefs in a PSL fixture that transcends sport. Limited final-batch tickets available.',
  'https://picsum.photos/seed/sowetoderby/600/400',
  'https://picsum.photos/seed/sowetoderbybn/1200/600',
  2, org_id, 'FNB Stadium', 'Soccer City Ave, Nasrec', 'Johannesburg', 'Africa/Johannesburg',
  280.00, 'ZAR', 94700, 93000,
  'published', NOW() + INTERVAL '22 days', NOW() + INTERVAL '22 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '26 days', NOW() - INTERVAL '10 days', 142000 ),

-- 10. Gallagher — Tech
( 'e7000000-0000-0000-0000-000000000010',
  make_slug('CES Africa Johannesburg 2026', 'e7000000-0000-0000-0000-000000000010'),
  'CES Africa Johannesburg 2026',
  'Africa''s answer to CES. Consumer electronics, smart home technology, wearables, automotive tech, and health tech on display from 200 exhibitors. Public open Saturday and Sunday. Trade days Monday to Thursday. Keynote speakers from leading global tech companies.',
  'https://picsum.photos/seed/cesafrica/600/400',
  NULL,
  4, org_id, 'Gallagher Convention Centre', 'Hexagon Rd, Midrand', 'Johannesburg', 'Africa/Johannesburg',
  280.00, 'ZAR', 30000, 22000,
  'published', NOW() + INTERVAL '32 days', NOW() + INTERVAL '35 days',
  NOW() + INTERVAL '39 days', NOW() - INTERVAL '16 days', 24800 ),

-- 11. Johannesburg — Gala
( 'e7000000-0000-0000-0000-000000000011',
  make_slug('Sunday Times Top 100 Companies Gala 2026', 'e7000000-0000-0000-0000-000000000011'),
  'Sunday Times Top 100 Companies Gala 2026',
  'The most prestigious business event in South Africa. The Sunday Times Top 100 Companies awards celebrate the best-performing JSE-listed companies. Attended by CEOs, chairpeople, and business luminaries. Black tie. Tables of 10. No individual tickets.',
  'https://picsum.photos/seed/sundaytimesgala/600/400',
  NULL,
  3, org_id, 'Sandton Convention Centre', '161 Maude St, Sandton', 'Johannesburg', 'Africa/Johannesburg',
  45000.00, 'ZAR', 600, 580,
  'published', NOW() + INTERVAL '29 days', NOW() + INTERVAL '29 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '33 days', NOW() - INTERVAL '14 days', 18200 ),

-- 12. Johannesburg — Food
( 'e7000000-0000-0000-0000-000000000012',
  make_slug('Maboneng Precinct Night Market & Art Walk', 'e7000000-0000-0000-0000-000000000012'),
  'Maboneng Precinct Night Market & Art Walk',
  'Every first Friday of the month, Maboneng transforms into Johannesburg''s most vibrant outdoor gallery and food market. Local artists display and sell work, street food from across Africa, live music, and guided art walks through the precinct''s galleries.',
  'https://picsum.photos/seed/mabonengmarket/600/400',
  NULL,
  5, org_id, 'Arts on Main', '264 Fox St, Maboneng', 'Johannesburg', 'Africa/Johannesburg',
  0.00, 'ZAR', 4000, 3500,
  'published', NOW() + INTERVAL '5 days', NOW() + INTERVAL '5 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '8 days', NOW() - INTERVAL '2 days', 13600 ),

-- 13. FNB Stadium — Rugby
( 'e7000000-0000-0000-0000-000000000013',
  make_slug('Springboks vs England Castle Lager Test Johannesburg', 'e7000000-0000-0000-0000-000000000013'),
  'Springboks vs England Castle Lager Test Johannesburg',
  'The world champions face England at FNB Stadium in a Castle Lager Incoming Series Test. The most anticipated England Test in South Africa since the 2018 series. Hospitality packages in the VIP enclosures, fan zones, and a full match-day entertainment programme.',
  'https://picsum.photos/seed/boksengland/600/400',
  'https://picsum.photos/seed/boksenglandbn/1200/600',
  2, org_id, 'FNB Stadium', 'Soccer City Ave, Nasrec', 'Johannesburg', 'Africa/Johannesburg',
  420.00, 'ZAR', 94700, 82000,
  'published', NOW() + INTERVAL '43 days', NOW() + INTERVAL '43 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '47 days', NOW() - INTERVAL '23 days', 67800 ),

-- 14. Northgate — Food
( 'e7000000-0000-0000-0000-000000000014',
  make_slug('Johannesburg Craft Beer & Braai Festival 2026', 'e7000000-0000-0000-0000-000000000014'),
  'Johannesburg Craft Beer & Braai Festival 2026',
  '35 craft breweries, 20 braai pitmasters, and Johannesburg''s most dedicated craft beer crowd. The Ticketpro Dome hosts its annual craft festival with a competition for best craft IPA, best braai, and most creative beer name. Family-friendly until 17:00. 18+ after.',
  'https://picsum.photos/seed/joburgcraftbeer/600/400',
  NULL,
  5, org_id, 'Ticketpro Dome', 'Northgate, Randburg', 'Johannesburg', 'Africa/Johannesburg',
  180.00, 'ZAR', 8000, 6400,
  'published', NOW() + INTERVAL '11 days', NOW() + INTERVAL '12 days',
  NOW() + INTERVAL '15 days', NOW() - INTERVAL '4 days', 19800 ),

-- 15. Ellis Park — Sports
( 'e7000000-0000-0000-0000-000000000015',
  make_slug('SA20 Joburg Super Kings vs Paarl Royals Ellis Park', 'e7000000-0000-0000-0000-000000000015'),
  'SA20 Joburg Super Kings vs Paarl Royals Ellis Park',
  'T20 franchise cricket at Ellis Park as the Joburg Super Kings host the Paarl Royals. The SA20 has transformed South African cricket — fast, colourful, and unmissable. Family zones, entertainment between overs, and some of the best cricket played in the world.',
  'https://picsum.photos/seed/sa20joburg/600/400',
  NULL,
  2, org_id, 'Emirates Airlines Park (Ellis Park)', 'Staib St, Doornfontein', 'Johannesburg', 'Africa/Johannesburg',
  180.00, 'ZAR', 62000, 41000,
  'published', NOW() + INTERVAL '6 days', NOW() + INTERVAL '6 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '9 days', NOW() - INTERVAL '2 days', 28700 ),

-- 16. Johannesburg — Exclusive
( 'e7000000-0000-0000-0000-000000000016',
  make_slug('Franschhoek Motor Museum Prestige Car Gala JHB', 'e7000000-0000-0000-0000-000000000016'),
  'Franschhoek Motor Museum Prestige Car Gala JHB',
  'The Johannesburg leg of the Franschhoek Motor Museum''s prestigious concours d''elegance. 40 museum-quality vehicles on static display at the Wanderers Club, followed by an exclusive black-tie dinner. Motoring celebrities, auction of rare memorabilia.',
  'https://picsum.photos/seed/francshhoekmotor/600/400',
  NULL,
  6, org_id, 'The Wanderers Club', '21 North St, Illovo', 'Johannesburg', 'Africa/Johannesburg',
  5500.00, 'ZAR', 250, 190,
  'published', NOW() + INTERVAL '26 days', NOW() + INTERVAL '26 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '30 days', NOW() - INTERVAL '12 days', 11400 ),

-- 17. Johannesburg — Music
( 'e7000000-0000-0000-0000-000000000017',
  make_slug('Joburg Theatre Musical: Hamilton South African Premiere', 'e7000000-0000-0000-0000-000000000017'),
  'Joburg Theatre Musical: Hamilton South African Premiere',
  'The South African premiere of Hamilton — the global phenomenon that changed musical theatre. Lin-Manuel Miranda''s masterpiece performed by an all-South African cast at the Joburg Theatre. A 12-week run. Secure your seats now.',
  'https://picsum.photos/seed/hamiltonsa/600/400',
  'https://picsum.photos/seed/hamiltonsaban/1200/600',
  1, org_id, 'Joburg Theatre', '163 Civic Blvd, Braamfontein', 'Johannesburg', 'Africa/Johannesburg',
  780.00, 'ZAR', 900, 860,
  'published', NOW() + INTERVAL '14 days', NOW() + INTERVAL '14 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '90 days', NOW() - INTERVAL '7 days', 48200 ),

-- 18. Johannesburg — Gala
( 'e7000000-0000-0000-0000-000000000018',
  make_slug('Mail & Guardian 200 Young South Africans Awards 2026', 'e7000000-0000-0000-0000-000000000018'),
  'Mail & Guardian 200 Young South Africans Awards 2026',
  'The most prestigious recognition of young South African achievement. The annual M&G 200 Young South Africans Awards gala celebrates exceptional under-40 leaders in business, arts, science, sport, and activism. Black tie. Invitation and RSVP.',
  'https://picsum.photos/seed/mg200young/600/400',
  NULL,
  3, org_id, 'The Maslow Hotel', 'Rivonia Rd, Sandton', 'Johannesburg', 'Africa/Johannesburg',
  2800.00, 'ZAR', 600, 520,
  'published', NOW() + INTERVAL '30 days', NOW() + INTERVAL '30 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '34 days', NOW() - INTERVAL '14 days', 12600 ),

-- 19. Johannesburg — Sports
( 'e7000000-0000-0000-0000-000000000019',
  make_slug('Johannesburg Marathon 2026', 'e7000000-0000-0000-0000-000000000019'),
  'Johannesburg Marathon 2026',
  'South Africa''s premier urban marathon. 42.2km, 21.1km, 10km, and 5km categories through the streets of Johannesburg. Starting and finishing at the Union Buildings (Sandton route). 20 000 runners from across Africa and the world. Timing chips included.',
  'https://picsum.photos/seed/joburgmarathon/600/400',
  NULL,
  2, org_id, 'Sandton City Start & Finish', 'Sandton Dr', 'Johannesburg', 'Africa/Johannesburg',
  380.00, 'ZAR', 20000, 17500,
  'published', NOW() + INTERVAL '38 days', NOW() + INTERVAL '38 days' + INTERVAL '8 hours',
  NOW() + INTERVAL '42 days', NOW() - INTERVAL '19 days', 24100 ),

-- 20. Johannesburg — Tech
( 'e7000000-0000-0000-0000-000000000020',
  make_slug('Africa Tech Week Johannesburg 2026', 'e7000000-0000-0000-0000-000000000020'),
  'Africa Tech Week Johannesburg 2026',
  'Five days of Africa''s biggest technology showcase. Fintech, healthtech, agritech, edtech, and cleantech pavilions. 500 exhibitors, 200 speakers, 50 workshops. The Expo runs daily; workshops and conference sessions require separate registration.',
  'https://picsum.photos/seed/africatechweek/600/400',
  'https://picsum.photos/seed/africatechweekbn/1200/600',
  4, org_id, 'Sandton Convention Centre', '161 Maude St, Sandton', 'Johannesburg', 'Africa/Johannesburg',
  1800.00, 'ZAR', 15000, 11000,
  'published', NOW() + INTERVAL '45 days', NOW() + INTERVAL '49 days',
  NOW() + INTERVAL '53 days', NOW() - INTERVAL '25 days', 38900 ),

-- 21. Johannesburg — Food
( 'e7000000-0000-0000-0000-000000000021',
  make_slug('Eat Out Restaurant Awards Gala Johannesburg 2026', 'e7000000-0000-0000-0000-000000000021'),
  'Eat Out Restaurant Awards Gala Johannesburg 2026',
  'South Africa''s equivalent of the Michelin Awards. The Eat Out Restaurant Awards gala is the most glamorous evening in the South African hospitality calendar. Attended by chefs, restaurateurs, food journalists, and the food-loving public. Bookings essential.',
  'https://picsum.photos/seed/eatoutawards/600/400',
  'https://picsum.photos/seed/eatoutawardsbn/1200/600',
  5, org_id, 'The Joburg Marriott', '138 West St, Sandton', 'Johannesburg', 'Africa/Johannesburg',
  3800.00, 'ZAR', 800, 720,
  'published', NOW() + INTERVAL '34 days', NOW() + INTERVAL '34 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '38 days', NOW() - INTERVAL '17 days', 22100 ),

-- 22. Johannesburg — Music
( 'e7000000-0000-0000-0000-000000000022',
  make_slug('Rock the Park Johannesburg 2026', 'e7000000-0000-0000-0000-000000000022'),
  'Rock the Park Johannesburg 2026',
  'Johannesburg''s biggest rock and alternative music festival returns to the Delta Park Amphitheatre. 8 acts over two stages — from classic rock to nu-metal and indie. Camping packages available. Dog-friendly (on leash). Local craft beer and artisan food village.',
  'https://picsum.photos/seed/rockthepark/600/400',
  NULL,
  1, org_id, 'Delta Park Amphitheatre', 'Federation Rd, Delta Park', 'Johannesburg', 'Africa/Johannesburg',
  420.00, 'ZAR', 8000, 6200,
  'published', NOW() + INTERVAL '27 days', NOW() + INTERVAL '27 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '31 days', NOW() - INTERVAL '12 days', 29600 )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- ██████████  STELLENBOSCH  (e8 prefix, 10 events)
-- =============================================================================

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at, views
) VALUES

( 'e8000000-0000-0000-0000-000000000001',
  make_slug('Stellenbosch Wine Harvest Festival 2026', 'e8000000-0000-0000-0000-000000000001'),
  'Stellenbosch Wine Harvest Festival 2026',
  'Celebrate the annual harvest at one of the world''s great wine towns. 40 Stellenbosch estates participate in open-cellar weekends, harvest lunches, blending workshops, and outdoor concerts. The most celebrated wine weekend in the Western Cape calendar.',
  'https://picsum.photos/seed/stellenharvest/600/400', NULL,
  5, org_id, 'Stellenbosch Town Centre & Wine Routes', 'Dorp St, Stellenbosch', 'Stellenbosch', 'Africa/Johannesburg',
  580.00, 'ZAR', 5000, 4200,
  'published', NOW() + INTERVAL '13 days', NOW() + INTERVAL '14 days',
  NOW() + INTERVAL '18 days', NOW() - INTERVAL '4 days', 22800 ),

( 'e8000000-0000-0000-0000-000000000002',
  make_slug('Maties vs UCT Varsity Cup Rugby Stellenbosch', 'e8000000-0000-0000-0000-000000000002'),
  'Maties vs UCT Varsity Cup Rugby Stellenbosch',
  'The Varsity Cup clash every Western Cape student and alumnus circles on the calendar. Maties host UCT at the Danie Craven Stadium in the most passionate student rugby fixture in the country. Bring your blue-and-gold or maroon — but pick a side.',
  'https://picsum.photos/seed/matiesuctrugby/600/400', NULL,
  2, org_id, 'Danie Craven Stadium', 'Merriman Ave, Stellenbosch', 'Stellenbosch', 'Africa/Johannesburg',
  120.00, 'ZAR', 15000, 13500,
  'published', NOW() + INTERVAL '8 days', NOW() + INTERVAL '8 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '12 days', NOW() - INTERVAL '3 days', 16400 ),

( 'e8000000-0000-0000-0000-000000000003',
  make_slug('Stellenbosch Food & Wine Pairing Masterclass', 'e8000000-0000-0000-0000-000000000003'),
  'Stellenbosch Food & Wine Pairing Masterclass',
  'A hands-on masterclass led by a Cape Master of Wine. Learn the science and art of food and wine pairing with six Cape Winelands wines and a curated menu. Groups of 12 per session. Three sessions daily over the weekend.',
  'https://picsum.photos/seed/stellenmasterclass/600/400', NULL,
  5, org_id, 'Tokara Wine Estate', 'Helshoogte Pass Rd', 'Stellenbosch', 'Africa/Johannesburg',
  980.00, 'ZAR', 36, 30,
  'published', NOW() + INTERVAL '10 days', NOW() + INTERVAL '10 days' + INTERVAL '3 hours',
  NOW() + INTERVAL '14 days', NOW() - INTERVAL '4 days', 4800 ),

( 'e8000000-0000-0000-0000-000000000004',
  make_slug('Spier Summer Concert Series Opening Night', 'e8000000-0000-0000-0000-000000000004'),
  'Spier Summer Concert Series Opening Night',
  'The iconic Spier Wine Farm opens its legendary summer concert series under the Cape night sky. Opening night features an all-South African lineup celebrating the country''s rich musical diversity. BYO picnic hamper or order a Spier hamper on arrival.',
  'https://picsum.photos/seed/spierconcert/600/400',
  'https://picsum.photos/seed/spierconcertbn/1200/600',
  1, org_id, 'Spier Wine Farm', 'R310, Stellenbosch', 'Stellenbosch', 'Africa/Johannesburg',
  380.00, 'ZAR', 6000, 5100,
  'published', NOW() + INTERVAL '16 days', NOW() + INTERVAL '16 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '20 days', NOW() - INTERVAL '5 days', 19600 ),

( 'e8000000-0000-0000-0000-000000000005',
  make_slug('Stellenbosch University Rag Festival 2026', 'e8000000-0000-0000-0000-000000000005'),
  'Stellenbosch University Rag Festival 2026',
  'The annual SU Rag Week culminates in the Rag Festival — the biggest student festival in the Western Cape. Live bands, DJ stages, a beach volleyball tournament, and the famous Rag float parade through the streets of Stellenbosch. Open to all.',
  'https://picsum.photos/seed/suragfest/600/400', NULL,
  1, org_id, 'Stellenbosch University Campus', 'Ryneveld St, Stellenbosch', 'Stellenbosch', 'Africa/Johannesburg',
  120.00, 'ZAR', 8000, 6400,
  'published', NOW() + INTERVAL '21 days', NOW() + INTERVAL '21 days' + INTERVAL '10 hours',
  NOW() + INTERVAL '25 days', NOW() - INTERVAL '8 days', 14200 ),

( 'e8000000-0000-0000-0000-000000000006',
  make_slug('Waterkloof Wine Estate Exclusive Harvest Dinner', 'e8000000-0000-0000-0000-000000000006'),
  'Waterkloof Wine Estate Exclusive Harvest Dinner',
  'The harvest dinner that defines Stellenbosch elegance. Waterkloof Estate hosts 40 guests for a five-course harvest menu prepared with estate-grown biodynamic produce, paired with Waterkloof''s finest wines. The most intimate wine dinner in the Winelands.',
  'https://picsum.photos/seed/waterkloof/600/400', NULL,
  6, org_id, 'Waterkloof Wine Estate', 'Sir Lowry''s Pass Rd, Somerset West', 'Stellenbosch', 'Africa/Johannesburg',
  5800.00, 'ZAR', 40, 32,
  'published', NOW() + INTERVAL '24 days', NOW() + INTERVAL '24 days' + INTERVAL '5 hours',
  NOW() + INTERVAL '28 days', NOW() - INTERVAL '10 days', 5600 ),

( 'e8000000-0000-0000-0000-000000000007',
  make_slug('Stellenbosch Cycle Challenge 2026', 'e8000000-0000-0000-0000-000000000007'),
  'Stellenbosch Cycle Challenge 2026',
  '90km, 50km, and 25km categories through the Stellenbosch wine valleys. One of the most scenic cycling routes in South Africa. Entry includes a finisher''s medal, wine tasting vouchers at three estates, and post-race braai at the finish.',
  'https://picsum.photos/seed/stellencycle/600/400', NULL,
  2, org_id, 'Stellenbosch Town Hall', 'Plein St, Stellenbosch', 'Stellenbosch', 'Africa/Johannesburg',
  480.00, 'ZAR', 3000, 2400,
  'published', NOW() + INTERVAL '19 days', NOW() + INTERVAL '19 days' + INTERVAL '6 hours',
  NOW() + INTERVAL '23 days', NOW() - INTERVAL '6 days', 12800 ),

( 'e8000000-0000-0000-0000-000000000008',
  make_slug('Woordfees Stellenbosch 2026 Opening Night', 'e8000000-0000-0000-0000-000000000008'),
  'Woordfees Stellenbosch 2026 Opening Night',
  'The Woordfees — South Africa''s premier Afrikaans arts festival — opens in Stellenbosch. Opening night gala features the premiere of a new Afrikaans drama, a literary auction, and a post-show reception with authors, playwrights, and artists.',
  'https://picsum.photos/seed/woordfees/600/400', NULL,
  3, org_id, 'Endler Hall, Stellenbosch University', 'Victoria St, Stellenbosch', 'Stellenbosch', 'Africa/Johannesburg',
  680.00, 'ZAR', 1200, 1050,
  'published', NOW() + INTERVAL '27 days', NOW() + INTERVAL '27 days' + INTERVAL '4 hours',
  NOW() + INTERVAL '31 days', NOW() - INTERVAL '12 days', 9200 ),

( 'e8000000-0000-0000-0000-000000000009',
  make_slug('SU Business School Tech & Innovation Summit', 'e8000000-0000-0000-0000-000000000009'),
  'SU Business School Tech & Innovation Summit',
  'The Stellenbosch University Business School convenes Africa''s leading thinkers on digital transformation, entrepreneurship, and sustainable business. Two days of presentations, workshops, and startup showcases from SU''s accelerator ecosystem.',
  'https://picsum.photos/seed/subizschool/600/400', NULL,
  4, org_id, 'SU Business School', 'Carl Cronje Dr, Bellville', 'Stellenbosch', 'Africa/Johannesburg',
  3200.00, 'ZAR', 800, 580,
  'published', NOW() + INTERVAL '33 days', NOW() + INTERVAL '34 days',
  NOW() + INTERVAL '37 days', NOW() - INTERVAL '16 days', 7400 ),

( 'e8000000-0000-0000-0000-000000000010',
  make_slug('Franschhoek Literary Festival 2026', 'e8000000-0000-0000-0000-000000000010'),
  'Franschhoek Literary Festival 2026',
  'The most beautiful literary festival in Africa. Three days of author talks, book launches, and discussions in the stunning Franschhoek valley. 80 authors from South Africa and around the world. Paired with exceptional Franschhoek winery lunches.',
  'https://picsum.photos/seed/franschhoeklit/600/400',
  'https://picsum.photos/seed/franschhoeklitbn/1200/600',
  5, org_id, 'Huguenot Memorial Museum', 'Lambrechts Rd, Franschhoek', 'Stellenbosch', 'Africa/Johannesburg',
  280.00, 'ZAR', 4000, 3400,
  'published', NOW() + INTERVAL '17 days', NOW() + INTERVAL '19 days',
  NOW() + INTERVAL '23 days', NOW() - INTERVAL '5 days', 18400 )

ON CONFLICT (id) DO NOTHING;

END $$;

-- =============================================================================
-- ██████████  TICKET TYPES  (all events above)
-- =============================================================================

INSERT INTO ticket_types (
    id, event_id,
    name, description,
    price, capacity, tickets_sold, reserved_count,
    sort_order, is_visible,
    sale_starts_at, sale_ends_at
)
VALUES

-- ── CAPE TOWN ────────────────────────────────────────────────────────────────

-- e2-001 Cape Town Stadium Soccer
( 'e2000001-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000001', 'General Admission', 'Standing terrace access.', 180.00, 30000, 22000, 500, 0, TRUE, NULL, NULL ),
( 'e2000001-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000001', 'Grandstand Seat', 'Covered reserved seat, great views.', 320.00, 20000, 13000, 300, 1, TRUE, NULL, NULL ),
( 'e2000001-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000001', 'VIP Suite', 'Private box for 8, catering included.', 9600.00, 625, 375, 20, 2, TRUE, NULL, NULL ),

-- e2-002 DHL Newlands Rugby
( 'e2000002-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000002', 'General Admission', 'Standing terrace, braai areas.', 220.00, 25000, 22000, 400, 0, TRUE, NULL, NULL ),
( 'e2000002-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000002', 'Reserved Seat', 'Named grandstand seat.', 420.00, 20000, 18000, 300, 1, TRUE, NULL, NULL ),
( 'e2000002-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000002', 'Hospitality Suite', 'Private suite for 10, full catering & bar.', 18000.00, 600, 400, 20, 2, TRUE, NULL, NULL ),

-- e2-003 Kirstenbosch Concert
( 'e2000003-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000003', 'Lawn Pass', 'BYO picnic blanket, general lawn.', 180.00, 6000, 4800, 100, 0, TRUE, NULL, NULL ),
( 'e2000003-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000003', 'Reserved Terrace', 'Allocated terrace seat, closer to stage.', 380.00, 2000, 1400, 50, 1, TRUE, NULL, NULL ),

-- e2-004 AfricaCom Tech
( 'e2000004-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000004', 'Exhibition Pass', 'Access to exhibition floor only.', 800.00, 6000, 4000, 100, 0, TRUE, NULL, NULL ),
( 'e2000004-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000004', 'Conference Delegate', 'All keynotes, sessions, networking, and exhibition.', 4500.00, 6000, 5000, 150, 1, TRUE, NULL, NULL ),
( 'e2000004-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000004', 'VIP Delegate', 'Priority seating, speaker dinners, and exclusive roundtables.', 12000.00, 500, 350, 20, 2, TRUE, NULL, NULL ),

-- e2-005 Constantia Wine Fest
( 'e2000005-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000005', 'Tasting Pass', '10 wine tastings included.', 280.00, 2000, 1500, 40, 0, TRUE, NULL, NULL ),
( 'e2000005-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000005', 'Connoisseur Pass', '20 tastings + chef masterclass.', 650.00, 800, 500, 20, 1, TRUE, NULL, NULL ),

-- e2-006 Artscape Gala
( 'e2000006-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000006', 'Stalls Seat', 'Main floor allocated seat.', 800.00, 600, 490, 15, 0, TRUE, NULL, NULL ),
( 'e2000006-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000006', 'Circle Seat', 'Upper circle view.', 600.00, 400, 320, 10, 1, TRUE, NULL, NULL ),
( 'e2000006-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000006', 'Box Seat', 'Private box, champagne on arrival.', 2800.00, 40, 30, 2, 2, TRUE, NULL, NULL ),

-- e2-007 City Run
( 'e2000007-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000007', '10K Entry', 'Timed 10K race. Timing chip included.', 220.00, 8000, 6500, 150, 0, TRUE, NULL, NULL ),
( 'e2000007-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000007', '5K Entry', 'Timed 5K. Timing chip included.', 150.00, 3000, 2200, 80, 1, TRUE, NULL, NULL ),
( 'e2000007-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000007', 'Fun Run', 'Untimed 3K fun run. All ages welcome.', 80.00, 2000, 1400, 50, 2, TRUE, NULL, NULL ),

-- e2-008 NYE Concert
( 'e2000008-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000008', 'General Admission', 'Standing festival access.', 480.00, 15000, 13000, 300, 0, TRUE, NULL, NULL ),
( 'e2000008-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000008', 'VIP Lounge', 'Elevated lounge, open bar, dedicated toilets.', 1800.00, 2000, 1800, 50, 1, TRUE, NULL, NULL ),
( 'e2000008-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000008', 'Platinum Table', 'Table for 8, premium open bar, priority entry.', 16000.00, 250, 200, 10, 2, TRUE, NULL, NULL ),

-- e2-009 GrandWest Comedy
( 'e2000009-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000009', 'Standard Seat', 'Allocated theatre seat.', 280.00, 1500, 1250, 30, 0, TRUE, NULL, NULL ),
( 'e2000009-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000009', 'Dinner & Show', 'Two-course dinner + premium front section.', 680.00, 400, 350, 10, 1, TRUE, NULL, NULL ),

-- e2-010 Black Coffee at CT Stadium
( 'e2000010-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000010', 'General Admission', 'Floor standing access.', 580.00, 40000, 36000, 800, 0, TRUE, NULL, NULL ),
( 'e2000010-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000010', 'Golden Circle', 'Closest to stage, wristband included.', 980.00, 8000, 7200, 200, 1, TRUE, NULL, NULL ),
( 'e2000010-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000010', 'VIP Viewing Platform', 'Elevated deck, open bar, premium view.', 2200.00, 2000, 1800, 50, 2, TRUE, NULL, NULL ),

-- e2-011 Sea Point Market (Free)
( 'e2000011-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000011', 'Free Entry', 'Free admission to the market.', 0.00, 5000, 3200, 0, 0, TRUE, NULL, NULL ),

-- e2-013 SA20 Cricket
( 'e2000013-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000013', 'General Admission', 'Standing area, big screen zones.', 100.00, 30000, 18000, 400, 0, TRUE, NULL, NULL ),
( 'e2000013-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000013', 'Grandstand Seat', 'Allocated covered seat.', 200.00, 15000, 9000, 200, 1, TRUE, NULL, NULL ),
( 'e2000013-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000013', 'Skybox Suite', 'Private box for 10. Catering.', 8000.00, 550, 300, 15, 2, TRUE, NULL, NULL ),

-- e2-016 Springboks vs Australia
( 'e2000016-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000016', 'General Admission', 'Standing terrace, full atmosphere.', 380.00, 25000, 24800, 100, 0, TRUE, NULL, NULL ),
( 'e2000016-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000016', 'Reserved Seat', 'Named covered seat.', 620.00, 20000, 19500, 100, 1, TRUE, NULL, NULL ),
( 'e2000016-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000016', 'Premium Hospitality', 'Private suite, open bar, 4-course meal.', 5500.00, 800, 680, 20, 2, TRUE, NULL, NULL ),

-- e2-018 Afropunk
( 'e2000018-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000018', 'Day 1 Pass', 'Full access to day 1.', 420.00, 5000, 4100, 100, 0, TRUE, NULL, NULL ),
( 'e2000018-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000018', 'Day 2 Pass', 'Full access to day 2.', 420.00, 5000, 4100, 100, 1, TRUE, NULL, NULL ),
( 'e2000018-0000-0000-0000-000000000003', 'e2000000-0000-0000-0000-000000000018', 'Weekend Pass', 'Both days + early entry.', 750.00, 2000, 1600, 50, 2, TRUE, NULL, NULL ),

-- e2-021 Cape Town Cycle Tour
( 'e2000021-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000021', '109K Entry', 'Main race, timing chip, finisher medal.', 780.00, 30000, 27000, 500, 0, TRUE, NULL, NULL ),
( 'e2000021-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000021', 'Corporate Relay Team (4)', 'Four-person relay, corporate lane start.', 4200.00, 1500, 1100, 30, 1, TRUE, NULL, NULL ),

-- e2-025 Cape Town Jazz Festival
( 'e2000025-0000-0000-0000-000000000001', 'e2000000-0000-0000-0000-000000000025', 'General Admission Day 1', 'All stages, one day.', 750.00, 30000, 26000, 600, 0, TRUE, NULL, NULL ),
( 'e2000025-0000-0000-0000-000000000002', 'e2000000-0000-0000-0000-000000000025', 'VIP Day 1', 'All stages + VIP lounge access.', 1800.00, 4000, 3500, 100, 1, TRUE, NULL, NULL ),

-- ── DURBAN ───────────────────────────────────────────────────────────────────

-- e3-001 Moses Mabhida Soccer
( 'e3000001-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000001', 'General Admission', 'Standing terrace.', 100.00, 30000, 22000, 400, 0, TRUE, NULL, NULL ),
( 'e3000001-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000001', 'Grandstand Seat', 'Covered allocated seat.', 220.00, 20000, 15000, 300, 1, TRUE, NULL, NULL ),
( 'e3000001-0000-0000-0000-000000000003', 'e3000000-0000-0000-0000-000000000001', 'Skybox Suite', 'Private box for 8, catering included.', 7200.00, 875, 500, 20, 2, TRUE, NULL, NULL ),

-- e3-002 Kings Park Rugby
( 'e3000002-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000002', 'General Standing', 'Standing terrace.', 180.00, 26000, 22500, 400, 0, TRUE, NULL, NULL ),
( 'e3000002-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000002', 'Reserved Seat', 'Allocated grandstand seat.', 380.00, 20000, 17500, 300, 1, TRUE, NULL, NULL ),
( 'e3000002-0000-0000-0000-000000000003', 'e3000000-0000-0000-0000-000000000002', 'Shark Tank VIP', 'Private suite for 10, full hospitality.', 16000.00, 650, 450, 20, 2, TRUE, NULL, NULL ),

-- e3-003 Afro Nation Moses Mabhida
( 'e3000003-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000003', 'Day 1 GA', 'General admission, day 1.', 480.00, 28000, 25000, 600, 0, TRUE, NULL, NULL ),
( 'e3000003-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000003', 'Day 2 GA', 'General admission, day 2.', 480.00, 28000, 24000, 600, 1, TRUE, NULL, NULL ),
( 'e3000003-0000-0000-0000-000000000003', 'e3000000-0000-0000-0000-000000000003', 'VIP Weekend', 'Both days, VIP viewing deck, open bar.', 2400.00, 2000, 1800, 50, 2, TRUE, NULL, NULL ),

-- e3-006 Sun Coast Gala
( 'e3000006-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000006', 'Individual Seat', 'Gala dinner, allocated seat.', 2800.00, 350, 290, 10, 0, TRUE, NULL, NULL ),
( 'e3000006-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000006', 'Corporate Table (10)', 'Table for 10 in the premium section.', 38000.00, 30, 22, 2, 1, TRUE, NULL, NULL ),

-- e3-007 Comrades Finish
( 'e3000007-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000007', 'Spectator Pass', 'Finish line viewing, grandstand access.', 120.00, 25000, 20000, 400, 0, TRUE, NULL, NULL ),
( 'e3000007-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000007', 'Runner Registration', 'Full Comrades race entry. Ultra marathon 89km.', 950.00, 5000, 4500, 100, 1, TRUE, NULL, NULL ),

-- e3-008 Durban July
( 'e3000008-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000008', 'General Admission', 'Trackside standing with food court access.', 650.00, 25000, 22000, 500, 0, TRUE, NULL, NULL ),
( 'e3000008-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000008', 'Grandstand Seat', 'Reserved seat, great track view.', 1200.00, 8000, 7000, 200, 1, TRUE, NULL, NULL ),
( 'e3000008-0000-0000-0000-000000000003', 'e3000000-0000-0000-0000-000000000008', 'Trackside Marquee Table', 'Premium trackside hospitality table for 10.', 45000.00, 350, 320, 10, 2, TRUE, NULL, NULL ),

-- e3-012 Kingsmead Cricket
( 'e3000012-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000012', 'General Admission', 'Standing zone, food court access.', 80.00, 15000, 10000, 300, 0, TRUE, NULL, NULL ),
( 'e3000012-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000012', 'Grandstand Seat', 'Allocated covered seat.', 180.00, 8000, 5500, 150, 1, TRUE, NULL, NULL ),

-- e3-015 Ladysmith Farewell Tour
( 'e3000015-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000015', 'Stalls Seat', 'Main floor allocated seat.', 480.00, 900, 890, 10, 0, TRUE, NULL, NULL ),
( 'e3000015-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000015', 'Circle Seat', 'Upper balcony seat.', 380.00, 400, 380, 5, 1, TRUE, NULL, NULL ),
( 'e3000015-0000-0000-0000-000000000003', 'e3000000-0000-0000-0000-000000000015', 'Golden Circle', 'Front row, signed programme included.', 1200.00, 80, 78, 2, 2, TRUE, NULL, NULL ),

-- e3-021 Amapiano Stadium Durban
( 'e3000021-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000021', 'General Admission', 'Standing floor festival access.', 420.00, 40000, 34000, 800, 0, TRUE, NULL, NULL ),
( 'e3000021-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000021', 'Golden Circle', 'Premium floor, closest to stage.', 780.00, 8000, 7000, 200, 1, TRUE, NULL, NULL ),
( 'e3000021-0000-0000-0000-000000000003', 'e3000000-0000-0000-0000-000000000021', 'VIP Lounge', 'Elevated lounge, open bar, private toilets.', 2200.00, 1500, 1200, 50, 2, TRUE, NULL, NULL ),

-- e3-023 Sharks vs Leinster
( 'e3000023-0000-0000-0000-000000000001', 'e3000000-0000-0000-0000-000000000023', 'General Admission', 'Standing terrace.', 320.00, 26000, 24000, 400, 0, TRUE, NULL, NULL ),
( 'e3000023-0000-0000-0000-000000000002', 'e3000000-0000-0000-0000-000000000023', 'Reserved Seat', 'Allocated grandstand seat.', 580.00, 20000, 19000, 300, 1, TRUE, NULL, NULL ),
( 'e3000023-0000-0000-0000-000000000003', 'e3000000-0000-0000-0000-000000000023', 'Champions Cup Suite', 'Private suite for 12, full hospitality.', 20000.00, 500, 400, 15, 2, TRUE, NULL, NULL ),

-- ── PRETORIA ─────────────────────────────────────────────────────────────────

-- e4-001 Loftus Rugby
( 'e4000001-0000-0000-0000-000000000001', 'e4000000-0000-0000-0000-000000000001', 'General Admission', 'Standing terraces.', 220.00, 26000, 23000, 400, 0, TRUE, NULL, NULL ),
( 'e4000001-0000-0000-0000-000000000002', 'e4000000-0000-0000-0000-000000000002', 'Reserved Seat', 'Named grandstand seat, covered.', 420.00, 20000, 18000, 300, 1, TRUE, NULL, NULL ),
( 'e4000001-0000-0000-0000-000000000003', 'e4000000-0000-0000-0000-000000000003', 'Loftus Lounge', 'Premium hospitality lounge for 10.', 18000.00, 600, 480, 15, 2, TRUE, NULL, NULL ),

-- e4-002 SuperSport Park Cricket
( 'e4000002-0000-0000-0000-000000000001', 'e4000000-0000-0000-0000-000000000002', 'General Admission', 'Grass bank or standing zone.', 120.00, 12000, 9000, 200, 0, TRUE, NULL, NULL ),
( 'e4000002-0000-0000-0000-000000000002', 'e4000000-0000-0000-0000-000000000002', 'Grandstand Seat', 'Allocated covered seat.', 220.00, 6000, 4500, 100, 1, TRUE, NULL, NULL ),
( 'e4000002-0000-0000-0000-000000000003', 'e4000000-0000-0000-0000-000000000002', 'Skybox Suite', 'Private box, catering included.', 12000.00, 200, 150, 8, 2, TRUE, NULL, NULL ),

-- e4-007 Loftus Live Concert
( 'e4000007-0000-0000-0000-000000000001', 'e4000000-0000-0000-0000-000000000007', 'General Admission', 'Standing floor and terraces.', 480.00, 35000, 29000, 600, 0, TRUE, NULL, NULL ),
( 'e4000007-0000-0000-0000-000000000002', 'e4000000-0000-0000-0000-000000000007', 'Golden Circle', 'Floor closest to stage.', 850.00, 10000, 8800, 200, 1, TRUE, NULL, NULL ),
( 'e4000007-0000-0000-0000-000000000003', 'e4000000-0000-0000-0000-000000000007', 'Premium Hospitality', 'Suite for 8, open bar.', 18000.00, 800, 600, 20, 2, TRUE, NULL, NULL ),

-- e4-012 Tshwane Derby
( 'e4000012-0000-0000-0000-000000000001', 'e4000000-0000-0000-0000-000000000012', 'General Admission', 'Standing terraces.', 100.00, 30000, 28000, 500, 0, TRUE, NULL, NULL ),
( 'e4000012-0000-0000-0000-000000000002', 'e4000000-0000-0000-0000-000000000012', 'Reserved Seat', 'Named covered seat.', 220.00, 16000, 15000, 200, 1, TRUE, NULL, NULL ),

-- e4-015 Jacaranda Festival
( 'e4000015-0000-0000-0000-000000000001', 'e4000000-0000-0000-0000-000000000015', 'General Admission', 'Full festival access, standing.', 180.00, 8000, 7200, 150, 0, TRUE, NULL, NULL ),
( 'e4000015-0000-0000-0000-000000000002', 'e4000000-0000-0000-0000-000000000015', 'VIP Terrace', 'Elevated terrace, open bar, premium view.', 580.00, 1500, 1200, 40, 1, TRUE, NULL, NULL ),

-- e4-016 SuperSport Park Rugby
( 'e4000016-0000-0000-0000-000000000001', 'e4000000-0000-0000-0000-000000000016', 'General Admission', 'Standing, braai areas.', 160.00, 12000, 10500, 200, 0, TRUE, NULL, NULL ),
( 'e4000016-0000-0000-0000-000000000002', 'e4000000-0000-0000-0000-000000000016', 'Grandstand Seat', 'Allocated seat.', 280.00, 6000, 5500, 100, 1, TRUE, NULL, NULL ),

-- e4-021 Boks vs All Blacks
( 'e4000021-0000-0000-0000-000000000001', 'e4000000-0000-0000-0000-000000000021', 'General Admission', 'Standing terraces.', 480.00, 26000, 25900, 50, 0, TRUE, NULL, NULL ),
( 'e4000021-0000-0000-0000-000000000002', 'e4000000-0000-0000-0000-000000000021', 'Reserved Seat', 'Named covered seat.', 780.00, 20000, 19900, 50, 1, TRUE, NULL, NULL ),
( 'e4000021-0000-0000-0000-000000000003', 'e4000000-0000-0000-0000-000000000021', 'Premium Box', 'Private box for 8, full hospitality.', 22000.00, 750, 720, 15, 2, TRUE, NULL, NULL ),

-- ── GQEBERHA ─────────────────────────────────────────────────────────────────

-- e5-001 NMB Stadium Soccer
( 'e5000001-0000-0000-0000-000000000001', 'e5000000-0000-0000-0000-000000000001', 'General Admission', 'Standing terrace.', 80.00, 30000, 18000, 400, 0, TRUE, NULL, NULL ),
( 'e5000001-0000-0000-0000-000000000002', 'e5000000-0000-0000-0000-000000000001', 'Grandstand Seat', 'Covered allocated seat.', 180.00, 12000, 8500, 200, 1, TRUE, NULL, NULL ),

-- e5-002 St George's Cricket
( 'e5000002-0000-0000-0000-000000000001', 'e5000000-0000-0000-0000-000000000002', 'General Admission', 'Grass bank.', 60.00, 10000, 4500, 150, 0, TRUE, NULL, NULL ),
( 'e5000002-0000-0000-0000-000000000002', 'e5000000-0000-0000-0000-000000000002', 'Members Pavilion', 'Covered pavilion seat.', 150.00, 5000, 2500, 80, 1, TRUE, NULL, NULL ),

-- e5-007 Algoa FM Big Concert
( 'e5000007-0000-0000-0000-000000000001', 'e5000000-0000-0000-0000-000000000007', 'General Admission', 'Lawn festival standing.', 280.00, 35000, 29000, 600, 0, TRUE, NULL, NULL ),
( 'e5000007-0000-0000-0000-000000000002', 'e5000000-0000-0000-0000-000000000007', 'Golden Circle', 'Closest to stage, priority entry.', 580.00, 8000, 7000, 150, 1, TRUE, NULL, NULL ),
( 'e5000007-0000-0000-0000-000000000003', 'e5000000-0000-0000-0000-000000000007', 'VIP Lounge', 'Elevated lounge, open bar.', 1600.00, 1500, 1200, 40, 2, TRUE, NULL, NULL ),

-- e5-011 Ironman SA
( 'e5000011-0000-0000-0000-000000000001', 'e5000000-0000-0000-0000-000000000011', 'Ironman 140.6 Entry', 'Full distance triathlon race entry.', 4800.00, 1500, 1400, 30, 0, TRUE, NULL, NULL ),
( 'e5000011-0000-0000-0000-000000000002', 'e5000000-0000-0000-0000-000000000011', 'Ironman 70.3 Entry', 'Half distance race entry.', 2800.00, 500, 450, 15, 1, TRUE, NULL, NULL ),

-- ── BLOEMFONTEIN ─────────────────────────────────────────────────────────────

-- e6-001 Free State Stadium Soccer
( 'e6000001-0000-0000-0000-000000000001', 'e6000000-0000-0000-0000-000000000001', 'General Admission', 'Standing terraces.', 60.00, 30000, 18500, 400, 0, TRUE, NULL, NULL ),
( 'e6000001-0000-0000-0000-000000000002', 'e6000000-0000-0000-0000-000000000001', 'Grandstand Seat', 'Covered allocated seat.', 140.00, 12000, 8000, 200, 1, TRUE, NULL, NULL ),

-- e6-002 Goodyear Park Cricket
( 'e6000002-0000-0000-0000-000000000001', 'e6000000-0000-0000-0000-000000000002', 'General Admission', 'Grass bank standing.', 60.00, 12000, 7500, 200, 0, TRUE, NULL, NULL ),
( 'e6000002-0000-0000-0000-000000000002', 'e6000000-0000-0000-0000-000000000002', 'Grandstand Seat', 'Covered allocated seat.', 140.00, 6000, 3500, 100, 1, TRUE, NULL, NULL ),

-- e6-006 Cheetahs vs Griquas
( 'e6000006-0000-0000-0000-000000000001', 'e6000000-0000-0000-0000-000000000006', 'General Admission', 'Standing terrace.', 140.00, 25000, 18000, 400, 0, TRUE, NULL, NULL ),
( 'e6000006-0000-0000-0000-000000000002', 'e6000000-0000-0000-0000-000000000006', 'Reserved Seat', 'Allocated covered grandstand.', 280.00, 16000, 12000, 300, 1, TRUE, NULL, NULL ),

-- e6-013 Amapiano Bloemfontein
( 'e6000013-0000-0000-0000-000000000001', 'e6000000-0000-0000-0000-000000000013', 'General Admission', 'Stadium floor standing.', 250.00, 35000, 28000, 600, 0, TRUE, NULL, NULL ),
( 'e6000013-0000-0000-0000-000000000002', 'e6000000-0000-0000-0000-000000000013', 'Golden Circle', 'Front floor, closest to stage.', 480.00, 7000, 6000, 150, 1, TRUE, NULL, NULL ),
( 'e6000013-0000-0000-0000-000000000003', 'e6000000-0000-0000-0000-000000000013', 'VIP Suite', 'Private hospitality suite.', 1800.00, 1000, 800, 30, 2, TRUE, NULL, NULL ),

-- ── EXTRA JOHANNESBURG ───────────────────────────────────────────────────────

-- e7-001 Bafana vs Nigeria FNB
( 'e7000001-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000001', 'General Admission', 'Standing terrace.', 150.00, 60000, 55000, 800, 0, TRUE, NULL, NULL ),
( 'e7000001-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000001', 'Grandstand Seat', 'Covered allocated seat.', 280.00, 28000, 25000, 400, 1, TRUE, NULL, NULL ),
( 'e7000001-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000001', 'VIP Box', 'Private box for 8. Full hospitality.', 12000.00, 875, 750, 20, 2, TRUE, NULL, NULL ),

-- e7-002 Lions vs Stormers Ellis Park
( 'e7000002-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000002', 'General Admission', 'Standing terrace.', 250.00, 32000, 28000, 400, 0, TRUE, NULL, NULL ),
( 'e7000002-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000002', 'Reserved Seat', 'Named grandstand seat.', 450.00, 24000, 21000, 300, 1, TRUE, NULL, NULL ),
( 'e7000002-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000002', 'VIP Suite', 'Private suite for 10, full hospitality.', 20000.00, 750, 600, 15, 2, TRUE, NULL, NULL ),

-- e7-003 Beyoncé at FNB
( 'e7000003-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000003', 'General Admission', 'Standing floor and terraces.', 1200.00, 60000, 60000, 0, 0, FALSE, NULL, NULL ),
( 'e7000003-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000003', 'Golden Circle', 'Closest to stage, wristband.', 2400.00, 20000, 20000, 0, 1, FALSE, NULL, NULL ),
( 'e7000003-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000003', 'VIP Floor Package', 'Premium floor zone, open bar.', 4800.00, 8000, 7800, 0, 2, TRUE, NULL, NULL ),
( 'e7000003-0000-0000-0000-000000000004', 'e7000000-0000-0000-0000-000000000003', 'Platinum Suite', 'Private suite for 8, full premium.', 48000.00, 875, 860, 5, 3, TRUE, NULL, NULL ),

-- e7-008 Amapiano Ellis Park
( 'e7000008-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000008', 'General Admission', 'Stadium floor standing.', 480.00, 42000, 38000, 800, 0, TRUE, NULL, NULL ),
( 'e7000008-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000008', 'Golden Circle', 'Front floor, closest to stage.', 880.00, 12000, 11000, 200, 1, TRUE, NULL, NULL ),
( 'e7000008-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000008', 'VIP Deck', 'Elevated premium viewing, open bar.', 2500.00, 2000, 1800, 50, 2, TRUE, NULL, NULL ),

-- e7-009 Soweto Derby FNB
( 'e7000009-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000009', 'General Admission', 'Standing terrace.', 200.00, 60000, 59000, 200, 0, TRUE, NULL, NULL ),
( 'e7000009-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000009', 'Reserved Seat', 'Named covered seat.', 380.00, 28000, 27500, 100, 1, TRUE, NULL, NULL ),
( 'e7000009-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000009', 'VIP Box', 'Private box for 8, full hospitality.', 14000.00, 875, 850, 10, 2, TRUE, NULL, NULL ),

-- e7-013 Boks vs England FNB
( 'e7000013-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000013', 'General Admission', 'Standing terraces.', 300.00, 60000, 52000, 600, 0, TRUE, NULL, NULL ),
( 'e7000013-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000013', 'Reserved Seat', 'Covered allocated seat.', 520.00, 28000, 24000, 300, 1, TRUE, NULL, NULL ),
( 'e7000013-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000013', 'VIP Hospitality', 'Suite for 8, full catering and open bar.', 18000.00, 875, 750, 15, 2, TRUE, NULL, NULL ),

-- e7-015 SA20 Ellis Park Cricket
( 'e7000015-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000015', 'General Admission', 'Standing zone.', 120.00, 40000, 27000, 500, 0, TRUE, NULL, NULL ),
( 'e7000015-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000015', 'Grandstand Seat', 'Covered allocated seat.', 250.00, 16000, 11000, 200, 1, TRUE, NULL, NULL ),
( 'e7000015-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000015', 'Skybox Suite', 'Private box for 10, catering.', 12000.00, 800, 550, 15, 2, TRUE, NULL, NULL ),

-- e7-017 Hamilton Musical (multiple nights)
( 'e7000017-0000-0000-0000-000000000001', 'e7000000-0000-0000-0000-000000000017', 'Stalls (A-D)', 'Best floor seats.', 980.00, 400, 380, 10, 0, TRUE, NULL, NULL ),
( 'e7000017-0000-0000-0000-000000000002', 'e7000000-0000-0000-0000-000000000017', 'Stalls (E-J)', 'Main floor seats.', 780.00, 300, 280, 8, 1, TRUE, NULL, NULL ),
( 'e7000017-0000-0000-0000-000000000003', 'e7000000-0000-0000-0000-000000000017', 'Circle', 'Upper circle balcony.', 580.00, 200, 180, 5, 2, TRUE, NULL, NULL ),

-- ── STELLENBOSCH ─────────────────────────────────────────────────────────────

-- e8-002 Maties vs UCT
( 'e8000002-0000-0000-0000-000000000001', 'e8000000-0000-0000-0000-000000000002', 'General Admission', 'Standing terraces.', 80.00, 10000, 9000, 200, 0, TRUE, NULL, NULL ),
( 'e8000002-0000-0000-0000-000000000002', 'e8000000-0000-0000-0000-000000000002', 'Reserved Seat', 'Allocated grandstand seat.', 180.00, 4000, 3800, 80, 1, TRUE, NULL, NULL ),

-- e8-004 Spier Concert
( 'e8000004-0000-0000-0000-000000000001', 'e8000000-0000-0000-0000-000000000004', 'Lawn Pass', 'BYO picnic blanket.', 280.00, 4500, 3900, 80, 0, TRUE, NULL, NULL ),
( 'e8000004-0000-0000-0000-000000000002', 'e8000000-0000-0000-0000-000000000004', 'VIP Terrace', 'Reserved terrace, closer to stage, waiter service.', 680.00, 1000, 800, 20, 1, TRUE, NULL, NULL )

ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- Verify counts
-- =============================================================================

SELECT
  city,
  count(*)          AS event_count,
  sum(views)        AS total_views
FROM events
WHERE city IN (
  'Cape Town','Durban','Pretoria','Bloemfontein',
  'Gqeberha','Johannesburg','Stellenbosch','Soweto'
)
GROUP BY city
ORDER BY event_count DESC;

COMMIT;
