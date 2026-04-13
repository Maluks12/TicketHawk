-- =============================================================================
-- seed_merch.sql — Test merch drops for the Drops screen
--
-- HOW TO RUN (from your events-db directory):
--   docker compose exec db psql -U devuser -d events_dev -f /seed_merch.sql
--
-- WHAT IT CREATES:
--   • 5 AVAILABLE NOW items  (sale_end in the future — shows countdown timer)
--   • 3 AVAILABLE  items     (no sale window — always live, like normal merch)
--   • 4 DROPPING SOON items  (sale_start in the future — shows countdown to drop)
--   • Variants (sizes/colours) for each item
--
-- SAFE TO RE-RUN: all inserts use ON CONFLICT DO NOTHING.
-- Uses the featured event from seed.sql (f1000000-0000-0000-0000-000000000001)
-- and the seed organizer (00000000-0000-0000-0000-000000000001).
-- =============================================================================

BEGIN;

-- ─── Helper: use the featured event from seed.sql ────────────────────────────
-- If you don't have it, a fallback event is created below.

INSERT INTO events (
  id, slug, title, description,
  cover_image_url, banner_image_url,
  category_id, organizer_id,
  venue_name, address, city, timezone,
  price, currency, max_capacity, tickets_sold,
  status, start_time, end_time, expires_at, published_at
) VALUES (
  'e0000000-0000-0000-0000-000000000099',
  'merch-drop-event-e0000000',
  'Merch Drop Event',
  'Placeholder event to anchor merch drops.',
  'https://picsum.photos/seed/merchevent/600/400',
  'https://picsum.photos/seed/mercheventbanner/1200/600',
  1,
  '00000000-0000-0000-0000-000000000001',
  'Online', 'Online', 'Johannesburg', 'Africa/Johannesburg',
  0.00, 'ZAR', 9999, 0,
  'published',
  NOW() + INTERVAL '90 days',
  NOW() + INTERVAL '91 days',
  NOW() + INTERVAL '92 days',
  NOW() - INTERVAL '1 day'
) ON CONFLICT (id) DO NOTHING;

-- ─── 1. AVAILABLE NOW — timed drops (sale_end in future) ─────────────────────
-- These show the "ENDS IN xx:xx:xx" countdown badge on the card.

-- 1a. Sneakers drop — ends in ~2 days (urgent — shows red timer)
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000001',
  'e0000000-0000-0000-0000-000000000099',
  'Air Invictus "Solar Flare" Low',
  'Limited colourway dropped exclusively for the Johannesburg leg. Gum outsole, reflective heel tab, and volt lace dubraes. Only 200 pairs allocated globally.',
  2499.00, 'sneakers', 1, TRUE,
  '["https://picsum.photos/seed/sneaker1a/600/600","https://picsum.photos/seed/sneaker1b/400/400","https://picsum.photos/seed/sneaker1c/400/400"]',
  NOW() - INTERVAL '1 day',
  NOW() + INTERVAL '2 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000011', 'aa000000-0000-0000-0000-000000000001', 'AIR-SF-UK8',  '{"size":"UK 8"}',  12, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000012', 'aa000000-0000-0000-0000-000000000001', 'AIR-SF-UK9',  '{"size":"UK 9"}',   5, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000013', 'aa000000-0000-0000-0000-000000000001', 'AIR-SF-UK10', '{"size":"UK 10"}',  2, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000014', 'aa000000-0000-0000-0000-000000000001', 'AIR-SF-UK11', '{"size":"UK 11"}',  0, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 1b. Apparel drop — ends in ~18 hours (urgent)
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000002',
  'e0000000-0000-0000-0000-000000000099',
  'Oversized Varsity Crewneck — Midnight',
  'Heavy 450gsm fleece with embroidered chest logo. Dropped at midnight, gone by sunrise. Sizes S–XL only.',
  899.00, 'apparel', 2, TRUE,
  '["https://picsum.photos/seed/crewneck1/600/600","https://picsum.photos/seed/crewneck2/400/400"]',
  NOW() - INTERVAL '6 hours',
  NOW() + INTERVAL '18 hours'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000021', 'aa000000-0000-0000-0000-000000000002', 'CREW-MID-S',  '{"size":"S"}',   8, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000022', 'aa000000-0000-0000-0000-000000000002', 'CREW-MID-M',  '{"size":"M"}',  15, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000023', 'aa000000-0000-0000-0000-000000000002', 'CREW-MID-L',  '{"size":"L"}',   9, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000024', 'aa000000-0000-0000-0000-000000000002', 'CREW-MID-XL', '{"size":"XL"}',  3, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 1c. Collectible — ends in ~5 days
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000003',
  'f1000000-0000-0000-0000-000000000001',
  'World Finals 2026 Official Match Ball',
  'Replica of the official tournament ball. Hand-stitched panels, thermally bonded seams. Numbered certificate of authenticity included. Limited to 500 units.',
  1250.00, 'collectible', 3, TRUE,
  '["https://picsum.photos/seed/ball2026/600/600"]',
  NOW() - INTERVAL '3 days',
  NOW() + INTERVAL '5 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000031', 'aa000000-0000-0000-0000-000000000003', 'BALL-2026-STD', '{"type":"Standard"}',  38, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000032', 'aa000000-0000-0000-0000-000000000003', 'BALL-2026-SIG', '{"type":"Signed"}',     5, 3500.00, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 1d. Jersey drop — ends in ~3 days
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000004',
  'f1000000-0000-0000-0000-000000000001',
  'Home Kit 2026 — Player Edition',
  'Authentic player-spec jersey, not the replica. Laser-cut ventilation, heat-transfer badge, slim fit. Allocated on a first-come basis.',
  1599.00, 'jersey', 4, TRUE,
  '["https://picsum.photos/seed/jersey2026h/600/600","https://picsum.photos/seed/jersey2026h2/400/400","https://picsum.photos/seed/jersey2026h3/400/400"]',
  NOW() - INTERVAL '12 hours',
  NOW() + INTERVAL '3 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000041', 'aa000000-0000-0000-0000-000000000004', 'HK26-PE-S',  '{"size":"S","fit":"player"}',  10, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000042', 'aa000000-0000-0000-0000-000000000004', 'HK26-PE-M',  '{"size":"M","fit":"player"}',  22, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000043', 'aa000000-0000-0000-0000-000000000004', 'HK26-PE-L',  '{"size":"L","fit":"player"}',  18, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000044', 'aa000000-0000-0000-0000-000000000004', 'HK26-PE-XL', '{"size":"XL","fit":"player"}',  7, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 1e. Footwear — ends in ~7 days
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000005',
  'e0000000-0000-0000-0000-000000000099',
  'Stadium Slide "Pitch Black"',
  'Post-match comfort in full black. EVA midsole, adjustable strap, water-resistant upper. Available while stock lasts.',
  450.00, 'footwear', 5, TRUE,
  '["https://picsum.photos/seed/slide2026/600/600","https://picsum.photos/seed/slide2026b/400/400"]',
  NOW() - INTERVAL '2 days',
  NOW() + INTERVAL '7 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000051', 'aa000000-0000-0000-0000-000000000005', 'SLIDE-PB-7',  '{"size":"UK 7"}',  20, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000052', 'aa000000-0000-0000-0000-000000000005', 'SLIDE-PB-8',  '{"size":"UK 8"}',  25, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000053', 'aa000000-0000-0000-0000-000000000005', 'SLIDE-PB-9',  '{"size":"UK 9"}',  18, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000054', 'aa000000-0000-0000-0000-000000000005', 'SLIDE-PB-10', '{"size":"UK 10"}', 12, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000055', 'aa000000-0000-0000-0000-000000000005', 'SLIDE-PB-11', '{"size":"UK 11"}',  6, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- ─── 2. AVAILABLE — no sale window (always live) ──────────────────────────────
-- These show no timer — they're just regular always-available merch.

-- 2a. Accessories
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000006',
  'e0000000-0000-0000-0000-000000000099',
  'Bucket Hat — Stadium Edition',
  '100% cotton twill, embroidered logo patch, adjustable draw cord. One size fits most.',
  350.00, 'accessories', 6, TRUE,
  '["https://picsum.photos/seed/buckhat1/600/600"]',
  NULL, NULL
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000061', 'aa000000-0000-0000-0000-000000000006', 'BHAT-OSFA', '{"size":"One Size"}', 60, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 2b. Culture
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000007',
  'e0000000-0000-0000-0000-000000000099',
  'Graphic Tee — "This Is Jozi"',
  'Screen-printed on 200gsm pre-shrunk cotton. Designed by a Soweto artist exclusively for this drop.',
  399.00, 'culture', 7, TRUE,
  '["https://picsum.photos/seed/jozitee1/600/600","https://picsum.photos/seed/jozitee2/400/400"]',
  NULL, NULL
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000071', 'aa000000-0000-0000-0000-000000000007', 'JOZI-TEE-S',  '{"size":"S"}',  30, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000072', 'aa000000-0000-0000-0000-000000000007', 'JOZI-TEE-M',  '{"size":"M"}',  45, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000073', 'aa000000-0000-0000-0000-000000000007', 'JOZI-TEE-L',  '{"size":"L"}',  40, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000074', 'aa000000-0000-0000-0000-000000000007', 'JOZI-TEE-XL', '{"size":"XL"}', 20, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 2c. Drops category
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000008',
  'e0000000-0000-0000-0000-000000000099',
  'Lanyard + Keyring Set',
  'Woven polyester lanyard with metal ID clip and matching enamel keyring. Standard drop item — always in stock.',
  120.00, 'drops', 8, TRUE,
  '["https://picsum.photos/seed/lanyard1/600/600"]',
  NULL, NULL
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000081', 'aa000000-0000-0000-0000-000000000008', 'LAN-SET-STD', '{"colour":"Black"}',  100, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000082', 'aa000000-0000-0000-0000-000000000008', 'LAN-SET-WHT', '{"colour":"White"}',   80, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- ─── 3. DROPPING SOON — sale_start in future ──────────────────────────────────
-- These appear in the "DROPPING SOON" tab with a countdown to release.

-- 3a. Drops in ~1 day
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000009',
  'e0000000-0000-0000-0000-000000000099',
  'Air Invictus "Volt" Mid — 2026 Edition',
  'The volt colourway returns. Reflective 3M detailing, carbon fibre shank, full-length foam cushioning. Drop confirmed at midnight.',
  3200.00, 'sneakers', 1, TRUE,
  '["https://picsum.photos/seed/airvolt1/600/600","https://picsum.photos/seed/airvolt2/400/400","https://picsum.photos/seed/airvolt3/400/400"]',
  NOW() + INTERVAL '1 day',
  NOW() + INTERVAL '4 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000091', 'aa000000-0000-0000-0000-000000000009', 'VOLT-UK8',  '{"size":"UK 8"}',  10, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000092', 'aa000000-0000-0000-0000-000000000009', 'VOLT-UK9',  '{"size":"UK 9"}',  10, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000093', 'aa000000-0000-0000-0000-000000000009', 'VOLT-UK10', '{"size":"UK 10"}', 10, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000094', 'aa000000-0000-0000-0000-000000000009', 'VOLT-UK11', '{"size":"UK 11"}', 10, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 3b. Drops in ~3 days
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000010',
  'f1000000-0000-0000-0000-000000000001',
  'Away Kit 2026 — Fan Edition',
  'The white-and-gold away kit worn in the group stage. Recycled polyester, slim fit, woven badge.',
  999.00, 'jersey', 2, TRUE,
  '["https://picsum.photos/seed/awaykit26/600/600","https://picsum.photos/seed/awaykit26b/400/400"]',
  NOW() + INTERVAL '3 days',
  NOW() + INTERVAL '10 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000101', 'aa000000-0000-0000-0000-000000000010', 'AK26-FE-S',  '{"size":"S"}',  50, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000102', 'aa000000-0000-0000-0000-000000000010', 'AK26-FE-M',  '{"size":"M"}',  80, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000103', 'aa000000-0000-0000-0000-000000000010', 'AK26-FE-L',  '{"size":"L"}',  70, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000104', 'aa000000-0000-0000-0000-000000000010', 'AK26-FE-XL', '{"size":"XL"}', 40, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 3c. Drops in ~5 days
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000011',
  'e0000000-0000-0000-0000-000000000099',
  'Tour Hoodie — "Black Ops" Edition',
  'Limited to 300 units. All-black embroidery, heavyweight 500gsm fleece, kangaroo pocket. Pre-order window only.',
  1100.00, 'apparel', 3, TRUE,
  '["https://picsum.photos/seed/blackops1/600/600","https://picsum.photos/seed/blackops2/400/400"]',
  NOW() + INTERVAL '5 days',
  NOW() + INTERVAL '8 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000111', 'aa000000-0000-0000-0000-000000000011', 'BOPS-S',  '{"size":"S"}',  30, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000112', 'aa000000-0000-0000-0000-000000000011', 'BOPS-M',  '{"size":"M"}',  50, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000113', 'aa000000-0000-0000-0000-000000000011', 'BOPS-L',  '{"size":"L"}',  60, NULL, TRUE),
  ('ab000000-0000-0000-0000-000000000114', 'aa000000-0000-0000-0000-000000000011', 'BOPS-XL', '{"size":"XL"}', 40, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

-- 3d. Drops in ~10 days
INSERT INTO merchandise (id, event_id, name, description, base_price, category, display_order, is_active, images, sale_start, sale_end)
VALUES (
  'aa000000-0000-0000-0000-000000000012',
  'f1000000-0000-0000-0000-000000000001',
  'Finals Commemorative Scarf',
  'Woven jacquard scarf with both team crests. Official licensed product. First drop ever for this category — collectors item.',
  299.00, 'collectible', 4, TRUE,
  '["https://picsum.photos/seed/scarf2026/600/600"]',
  NOW() + INTERVAL '10 days',
  NOW() + INTERVAL '17 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO merchandise_variants (id, merchandise_id, sku, attributes, stock_quantity, price_override, is_active)
VALUES
  ('ab000000-0000-0000-0000-000000000121', 'aa000000-0000-0000-0000-000000000012', 'SCARF-STD', '{"type":"Standard"}', 200, NULL, TRUE)
ON CONFLICT (id) DO NOTHING;

COMMIT;

-- ─── Verify ───────────────────────────────────────────────────────────────────
SELECT
  m.name,
  m.category,
  CASE
    WHEN m.sale_start IS NOT NULL AND m.sale_start > NOW() THEN 'DROPPING SOON'
    WHEN m.sale_end IS NOT NULL AND m.sale_end < NOW()     THEN 'ENDED'
    ELSE 'AVAILABLE NOW'
  END AS status,
  m.sale_start,
  m.sale_end,
  COUNT(v.id) AS variant_count
FROM merchandise m
LEFT JOIN merchandise_variants v ON v.merchandise_id = m.id AND v.is_active = TRUE
WHERE m.id::text LIKE 'aa000000%'
GROUP BY m.id, m.name, m.category, m.sale_start, m.sale_end
ORDER BY status, m.display_order;