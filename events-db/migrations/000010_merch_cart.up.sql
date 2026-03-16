-- Add reserved_count to events for capacity management
ALTER TABLE events ADD COLUMN IF NOT EXISTS reserved_count INTEGER NOT NULL DEFAULT 0;
 
-- Enum for cart item types
DO $$ BEGIN
  CREATE TYPE cart_item_type AS ENUM ('ticket', 'merchandise');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
 
-- Carts (one per user OR per guest session)
CREATE TABLE carts (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id          UUID REFERENCES users(id) ON DELETE CASCADE,
  session_id       UUID,
  last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT carts_owner_check CHECK (
    (user_id IS NOT NULL AND session_id IS NULL) OR
    (user_id IS NULL     AND session_id IS NOT NULL)
  )
);
CREATE INDEX idx_carts_user_id    ON carts(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_carts_session_id ON carts(session_id) WHERE session_id IS NOT NULL;
 
-- Merchandise (event-scoped)
CREATE TABLE merchandise (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  name          VARCHAR(255) NOT NULL,
  description   TEXT,
  base_price    NUMERIC(10,2) NOT NULL,
  category      VARCHAR(100),
  display_order INTEGER NOT NULL DEFAULT 0,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  images        JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_merch_event_id ON merchandise(event_id);
 
-- Merchandise variants (size, colour, etc.)
CREATE TABLE merchandise_variants (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  merchandise_id UUID NOT NULL REFERENCES merchandise(id) ON DELETE CASCADE,
  sku            VARCHAR(100) UNIQUE NOT NULL,
  attributes     JSONB NOT NULL DEFAULT '{}',
  stock_quantity INTEGER NOT NULL DEFAULT 0,
  price_override NUMERIC(10,2),
  is_active      BOOLEAN NOT NULL DEFAULT TRUE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT stock_non_negative CHECK (stock_quantity >= 0)
);
CREATE INDEX idx_variants_merch_id ON merchandise_variants(merchandise_id);
-- Cart items
CREATE TABLE cart_items (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cart_id               UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
  item_type             cart_item_type NOT NULL,
  event_id              UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  merchandise_variant_id UUID REFERENCES merchandise_variants(id) ON DELETE SET NULL,
  quantity              INTEGER NOT NULL DEFAULT 1,
  unit_price_snapshot   NUMERIC(10,2) NOT NULL,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT quantity_positive CHECK (quantity > 0),
  CONSTRAINT merch_requires_variant CHECK (
    (item_type = 'ticket'      AND merchandise_variant_id IS NULL) OR
    (item_type = 'merchandise' AND merchandise_variant_id IS NOT NULL)
  )
);
CREATE INDEX idx_cart_items_cart_id ON cart_items(cart_id);
