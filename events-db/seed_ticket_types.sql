-- ============================================================
-- seed_ticket_types.sql
-- Ticket tiers for every event in seed.sql
-- Run this AFTER seed.sql has been applied.
-- ============================================================

-- Helper: we use fixed UUIDs so re-runs are idempotent via ON CONFLICT DO NOTHING.

INSERT INTO ticket_types (
    id, event_id,
    name, description,
    price, capacity, tickets_sold, reserved_count,
    sort_order, is_visible,
    sale_starts_at, sale_ends_at
)
VALUES

-- ──────────────────────────────────────────────────────────────
-- f1000000-…-0001  |  The World Finals 2026  (Sports)
-- ──────────────────────────────────────────────────────────────
( 'f1000001-0000-0000-0000-000000000001', 'f1000000-0000-0000-0000-000000000001',
  'General Admission', 'Standing fan section. Access to all general concourses.',
  850.00, 5000, 1200, 80, 0, TRUE, NULL, NULL ),

( 'f1000001-0000-0000-0000-000000000002', 'f1000000-0000-0000-0000-000000000001',
  'Premium Seating', 'Covered grandstand seats with unobstructed views.',
  1800.00, 2000, 650, 30, 1, TRUE, NULL, NULL ),

( 'f1000001-0000-0000-0000-000000000003', 'f1000000-0000-0000-0000-000000000001',
  'VIP Hospitality', 'Private lounge, open bar, pre-match entertainment & premium seat.',
  4500.00, 300, 90, 10, 2, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0001  |  After Hours: Electronic Pulse  (Music)
-- ──────────────────────────────────────────────────────────────
( 'a1000001-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000001',
  'Early Bird', 'Limited early-bird release. Standing.',
  250.00, 200, 200, 0, 0, FALSE, NULL, NULL ),   -- sold out tier, hidden

( 'a1000001-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000001',
  'General Admission', 'Full night access. Standing on the floor.',
  350.00, 800, 310, 20, 1, TRUE, NULL, NULL ),

( 'a1000001-0000-0000-0000-000000000003', 'a1000000-0000-0000-0000-000000000001',
  'VIP Table', 'Reserved table for 4, waitress service, dedicated entrance.',
  1200.00, 50, 12, 2, 2, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0002  |  The Future of AI: Private Gala  (Tech / Conference)
-- ──────────────────────────────────────────────────────────────
( 'a1000002-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000002',
  'Delegate Pass', 'Full-day access to all talks, workshops, and networking lunch.',
  2500.00, 400, 180, 15, 0, TRUE, NULL, NULL ),

( 'a1000002-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000002',
  'Founders Table', 'Priority seating, dinner with keynote speakers, and gift bag.',
  7500.00, 40, 22, 3, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0003  |  Neon Pulse Festival  (Music)
-- ──────────────────────────────────────────────────────────────
( 'a1000003-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000003',
  'Day Pass', 'Access to one day of your choice.',
  400.00, 2000, 850, 40, 0, TRUE, NULL, NULL ),

( 'a1000003-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000003',
  'Weekend Pass', 'All-weekend access to every stage.',
  700.00, 1500, 620, 35, 1, TRUE, NULL, NULL ),

( 'a1000003-0000-0000-0000-000000000003', 'a1000000-0000-0000-0000-000000000003',
  'VIP Weekend', 'Weekend access + VIP lounge, artist meet & greet, and free merchandise.',
  1800.00, 200, 55, 8, 2, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0004  |  The Midnight Affair  (Nightlife)
-- ──────────────────────────────────────────────────────────────
( 'a1000004-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000004',
  'General Admission', 'Entry from 22:00. Standing.',
  300.00, 600, 250, 18, 0, TRUE, NULL, NULL ),

( 'a1000004-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000004',
  'Bottle-Service Table', 'Table for 6, one bottle included, priority entry.',
  3600.00, 30, 10, 2, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0005  |  Championship Sprint Final  (Sports)
-- ──────────────────────────────────────────────────────────────
( 'a1000005-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000005',
  'Grandstand A', 'Main straight grandstand — best overtaking views.',
  950.00, 3000, 1400, 60, 0, TRUE, NULL, NULL ),

( 'a1000005-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000005',
  'Grandstand B', 'Turn 1 grandstand — ideal for start/finish action.',
  750.00, 3000, 900, 50, 1, TRUE, NULL, NULL ),

( 'a1000005-0000-0000-0000-000000000003', 'a1000000-0000-0000-0000-000000000005',
  'Paddock Club', 'Exclusive paddock access, chef-prepared meals, and pit-lane walk.',
  8500.00, 150, 60, 5, 2, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0006  |  Black Tie Masquerade  (Gala)
-- ──────────────────────────────────────────────────────────────
( 'a1000006-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000006',
  'General Admission', 'Black-tie entry, cocktails on arrival, dancing until late.',
  550.00, 500, 200, 25, 0, TRUE, NULL, NULL ),

( 'a1000006-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000006',
  'Premium Table (10 pax)', 'Reserved table for 10, 3-course dinner, complimentary wine.',
  7500.00, 20, 8, 1, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0007  |  Global Founders Summit  (Business)
-- ──────────────────────────────────────────────────────────────
( 'a1000007-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000007',
  'Standard Delegate', 'Full conference access and networking sessions.',
  3500.00, 300, 110, 10, 0, TRUE, NULL, NULL ),

( 'a1000007-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000007',
  'Investor Pass', 'All sessions + curated 1-on-1 pitch meetings and investor dinner.',
  12000.00, 50, 18, 2, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0008  |  Gourmet Garden Festival  (Food & Wine)
-- ──────────────────────────────────────────────────────────────
( 'a1000008-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000008',
  'Tasting Pass', '10 curated food & wine tastings included.',
  380.00, 1200, 490, 30, 0, TRUE, NULL, NULL ),

( 'a1000008-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000008',
  'Connoisseur Pass', '20 tastings + masterclass seat + sommelier-guided tour.',
  900.00, 300, 85, 12, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0009  |  Wimbledon Finals Night  (Sports Viewing)
-- ──────────────────────────────────────────────────────────────
( 'a1000009-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000009',
  'Standing Area', 'Big-screen viewing. Bar access.',
  200.00, 500, 190, 15, 0, TRUE, NULL, NULL ),

( 'a1000009-0000-0000-0000-000000000002', 'a1000000-0000-0000-0000-000000000009',
  'Seated Lounge', 'Comfortable seating, waiter service, full menu.',
  550.00, 200, 60, 8, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- a1000000-…-0010  |  The Indie Sessions  (Music)
-- ──────────────────────────────────────────────────────────────
( 'a1000010-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000010',
  'General Admission', 'Standing floor ticket.',
  180.00, 400, 130, 10, 0, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- b1000000-…-0001  |  Monaco Paddock Club Jhb Viewing  (Sports)
-- ──────────────────────────────────────────────────────────────
( 'b1000001-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001',
  'Standard Seat', 'Reserved seat with screen view, bar access.',
  650.00, 300, 120, 10, 0, TRUE, NULL, NULL ),

( 'b1000001-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000001',
  'Paddock Experience', 'Premium lounge, live commentary, full hospitality.',
  1800.00, 80, 30, 5, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- b1000000-…-0002  |  The Clash in Vegas Jhb Watch Party  (Sports)
-- ──────────────────────────────────────────────────────────────
( 'b1000002-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000002',
  'General Entry', 'Big-screen access, bar open.',
  280.00, 600, 220, 20, 0, TRUE, NULL, NULL ),

( 'b1000002-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000002',
  'Ring-Side Table', 'Reserved table for 4, dedicated screen view, bottle package.',
  2200.00, 40, 14, 2, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- b1000000-…-0003  |  Jazz Under the Joburg Stars  (Music)
-- ──────────────────────────────────────────────────────────────
( 'b1000003-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000003',
  'Lawn Pass', 'BYO picnic blanket. General lawn seating.',
  220.00, 800, 310, 25, 0, TRUE, NULL, NULL ),

( 'b1000003-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000003',
  'Table Seating', 'Reserved table for 2, dinner service available.',
  480.00, 200, 70, 8, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- c1000000-…-0001  |  Underground Beats Weekender  (Music)
-- ──────────────────────────────────────────────────────────────
( 'c1000001-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000001',
  'Night Pass (Fri or Sat)', 'Single-night access.',
  320.00, 1000, 400, 30, 0, TRUE, NULL, NULL ),

( 'c1000001-0000-0000-0000-000000000002', 'c1000000-0000-0000-0000-000000000001',
  'Full Weekender', 'Both nights, early entry, backstage tour.',
  550.00, 500, 190, 20, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- c1000000-…-0002  |  Art Basel After Party Johannesburg  (Nightlife / Art)
-- ──────────────────────────────────────────────────────────────
( 'c1000002-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000002',
  'General Admission', 'Entry + 1 welcome cocktail.',
  450.00, 400, 175, 15, 0, TRUE, NULL, NULL ),

( 'c1000002-0000-0000-0000-000000000002', 'c1000000-0000-0000-0000-000000000002',
  'Artist Circle', 'Exclusive artist-interaction zone, open bar, collector preview.',
  1500.00, 60, 20, 4, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- c1000000-…-0003  |  Comedy Night Live Johannesburg  (Comedy)
-- ──────────────────────────────────────────────────────────────
( 'c1000003-0000-0000-0000-000000000001', 'c1000000-0000-0000-0000-000000000003',
  'General Admission', 'Allocated seating, two-drink minimum included.',
  220.00, 350, 140, 10, 0, TRUE, NULL, NULL ),

( 'c1000003-0000-0000-0000-000000000002', 'c1000000-0000-0000-0000-000000000003',
  'Front Row Experience', 'Front 3 rows, meet & greet with comedians after show.',
  600.00, 40, 15, 2, 1, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- d1000000-…-0001  |  Pro League Finals  (Esports / Sports)
-- ──────────────────────────────────────────────────────────────
( 'd1000001-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000001',
  'General Admission', 'Standing floor, big screens, food court access.',
  300.00, 3000, 1100, 80, 0, TRUE, NULL, NULL ),

( 'd1000001-0000-0000-0000-000000000002', 'd1000000-0000-0000-0000-000000000001',
  'Reserved Seat', 'Named seat in the main arena.',
  600.00, 1500, 500, 40, 1, TRUE, NULL, NULL ),

( 'd1000001-0000-0000-0000-000000000003', 'd1000000-0000-0000-0000-000000000001',
  'VIP Fan Suite', 'Private suite for 8, catering included, player autograph session.',
  6400.00, 20, 8, 1, 2, TRUE, NULL, NULL ),

-- ──────────────────────────────────────────────────────────────
-- d1000000-…-0002  |  Gentlemen's Singles Final View  (Sports Viewing)
-- ──────────────────────────────────────────────────────────────
( 'd1000002-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000002',
  'Lawn Viewing', 'Outdoor big-screen experience, picnic-style.',
  180.00, 600, 200, 15, 0, TRUE, NULL, NULL ),

( 'd1000002-0000-0000-0000-000000000002', 'd1000000-0000-0000-0000-000000000002',
  'Hospitality Lounge', 'Indoor lounge, full afternoon-tea service, private screens.',
  750.00, 120, 40, 5, 1, TRUE, NULL, NULL )

ON CONFLICT (id) DO NOTHING;