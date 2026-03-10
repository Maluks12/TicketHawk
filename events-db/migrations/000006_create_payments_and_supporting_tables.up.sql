-- migrations/000006_create_payments_and_supporting_tables.up.sql

-- Payments table
CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE RESTRICT,
    
    -- Payment gateway information
    payment_gateway VARCHAR(50) NOT NULL,
    payment_intent_id VARCHAR(255),
    transaction_id VARCHAR(255),
    
    -- Payment details
    amount DECIMAL(10,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'ZAR' NOT NULL,
    payment_method payment_method_type NOT NULL,
    card_last_4 VARCHAR(4),
    card_brand VARCHAR(20),
    
    -- Status and metadata
    status payment_status DEFAULT 'pending' NOT NULL,
    failure_reason TEXT,
    failure_code VARCHAR(50),
    
    -- Stripe-specific
    stripe_customer_id VARCHAR(255),
    stripe_payment_method_id VARCHAR(255),
    
    -- Refund tracking
    refund_id VARCHAR(255),
    refunded_amount DECIMAL(10,2),
    refunded_at TIMESTAMP WITH TIME ZONE,
    
    -- Webhooks
    webhook_received BOOLEAN DEFAULT FALSE,
    webhook_data JSONB,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT chk_payment_amount_positive CHECK (amount > 0),
    CONSTRAINT chk_refund_amount_valid CHECK (
        refunded_amount IS NULL OR 
        (refunded_amount > 0 AND refunded_amount <= amount)
    )
);

CREATE INDEX idx_payments_booking_id ON payments(booking_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_payment_intent ON payments(payment_intent_id);
CREATE INDEX idx_payments_transaction_id ON payments(transaction_id);
CREATE INDEX idx_payments_created_at ON payments(created_at DESC);
CREATE INDEX idx_payments_stripe_customer ON payments(stripe_customer_id);

CREATE TRIGGER update_payments_updated_at 
    BEFORE UPDATE ON payments
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Waitlist table
CREATE TABLE waitlist (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Waitlist details
    position INTEGER,
    requested_quantity INTEGER NOT NULL CHECK (requested_quantity > 0),
    notified BOOLEAN DEFAULT FALSE,
    notification_sent_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    
    -- Status
    status VARCHAR(50) DEFAULT 'active',
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT unique_user_event_waitlist UNIQUE(event_id, user_id)
);

CREATE INDEX idx_waitlist_event_id ON waitlist(event_id);
CREATE INDEX idx_waitlist_user_id ON waitlist(user_id);
CREATE INDEX idx_waitlist_status ON waitlist(status);
CREATE INDEX idx_waitlist_created_at ON waitlist(created_at);

CREATE TRIGGER update_waitlist_updated_at 
    BEFORE UPDATE ON waitlist
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Event views tracking
CREATE TABLE event_views (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ip_address INET,
    user_agent TEXT,
    referrer TEXT,
    viewed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    session_id VARCHAR(255)
);

CREATE INDEX idx_event_views_event_id ON event_views(event_id);
CREATE INDEX idx_event_views_user_id ON event_views(user_id);
CREATE INDEX idx_event_views_viewed_at ON event_views(viewed_at);

-- Revenue summary materialized view
CREATE MATERIALIZED VIEW event_revenue_summary AS
SELECT 
    e.id as event_id,
    e.title,
    e.organizer_id,
    COUNT(DISTINCT b.id) as total_bookings,
    COALESCE(SUM(b.ticket_quantity), 0) as total_tickets_sold,
    COALESCE(SUM(b.subtotal), 0) as gross_revenue,
    COALESCE(SUM(b.platform_fee), 0) as platform_fees,
    COALESCE(SUM(b.organizer_fee), 0) as organizer_fees,
    COALESCE(SUM(b.total_amount), 0) as total_revenue,
    COALESCE(SUM(CASE WHEN b.booking_status = 'refunded' THEN b.refund_amount ELSE 0 END), 0) as total_refunds,
    COALESCE(SUM(b.total_amount), 0) - COALESCE(SUM(CASE WHEN b.booking_status = 'refunded' THEN b.refund_amount ELSE 0 END), 0) as net_revenue
FROM events e
LEFT JOIN bookings b ON e.id = b.event_id 
    AND b.payment_status = 'completed'
GROUP BY e.id, e.title, e.organizer_id;

CREATE UNIQUE INDEX idx_event_revenue_event_id ON event_revenue_summary(event_id);

-- Function to refresh materialized view
CREATE OR REPLACE FUNCTION refresh_event_revenue_summary()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY event_revenue_summary;
END;
$$ LANGUAGE plpgsql;

-- Attendee demographics table (for analytics)
CREATE TABLE attendee_demographics (
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    age_group VARCHAR(20),
    city VARCHAR(255),
    booking_source VARCHAR(50),
    ticket_count INTEGER,
    created_at DATE DEFAULT CURRENT_DATE,
    
    PRIMARY KEY (event_id, age_group, city, created_at)
);

CREATE INDEX idx_demographics_event_id ON attendee_demographics(event_id);