CREATE TABLE ticket_reservations (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cart_id        UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
  event_id       UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
  session_id     UUID,
  booking_number VARCHAR(50) UNIQUE NOT NULL
                 DEFAULT 'BK-' || upper(substr(gen_random_uuid()::text, 1, 8)),
  quantity       INTEGER NOT NULL,
  expires_at     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '10 minutes',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT reservation_quantity_positive CHECK (quantity > 0)
);
CREATE INDEX idx_reservations_cart_id   ON ticket_reservations(cart_id);
CREATE INDEX idx_reservations_event_id  ON ticket_reservations(event_id);
CREATE INDEX idx_reservations_expires   ON ticket_reservations(expires_at);
