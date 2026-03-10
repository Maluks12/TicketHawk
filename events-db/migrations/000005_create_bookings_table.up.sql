-- migrations/000005_create_bookings_table.up.sql

CREATE TABLE bookings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    booking_number VARCHAR(20) UNIQUE NOT NULL,
    
    -- References
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    
    -- Ticket information
    ticket_quantity INTEGER NOT NULL CHECK (ticket_quantity > 0),
    unit_price DECIMAL(10,2) NOT NULL,
    subtotal DECIMAL(10,2) NOT NULL,
    platform_fee DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    organizer_fee DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    total_amount DECIMAL(10,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'ZAR' NOT NULL,
    
    -- Attendee information (can differ from user)
    attendee_first_name VARCHAR(100),
    attendee_last_name VARCHAR(100),
    attendee_email VARCHAR(255),
    attendee_phone VARCHAR(20),
    
    -- Status
    booking_status booking_status DEFAULT 'pending' NOT NULL,
    payment_status payment_status DEFAULT 'pending' NOT NULL,
    
    -- Tickets
    tickets_sent BOOLEAN DEFAULT FALSE,
    qr_code_data TEXT,
    qr_code_url TEXT,
    
    -- Refund information
    refund_requested_at TIMESTAMP WITH TIME ZONE,
    refund_reason TEXT,
    refunded_at TIMESTAMP WITH TIME ZONE,
    refund_amount DECIMAL(10,2),
    
    -- Metadata
    booking_notes TEXT,
    special_requirements TEXT,
    promo_code VARCHAR(50),
    discount_amount DECIMAL(10,2) DEFAULT 0.00,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    confirmed_at TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    
    -- Constraints
    CONSTRAINT chk_positive_amounts CHECK (
        subtotal >= 0 AND 
        total_amount >= 0 AND 
        platform_fee >= 0 AND 
        organizer_fee >= 0
    ),
    CONSTRAINT chk_total_calculation CHECK (
        total_amount = subtotal + platform_fee + organizer_fee - discount_amount
    )
);

-- Function to generate booking numbers
CREATE OR REPLACE FUNCTION generate_booking_number() RETURNS TEXT AS $$
DECLARE
    new_number TEXT;
    done BOOL;
BEGIN
    done := FALSE;
    WHILE NOT done LOOP
        new_number := 'BK-' || TO_CHAR(CURRENT_DATE, 'YYYYMMDD') || '-' || 
                     UPPER(SUBSTRING(MD5(RANDOM()::TEXT) FROM 1 FOR 6));
        done := NOT EXISTS(SELECT 1 FROM bookings WHERE booking_number = new_number);
    END LOOP;
    RETURN new_number;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-generate booking number
CREATE OR REPLACE FUNCTION set_booking_number() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.booking_number IS NULL OR NEW.booking_number = '' THEN
        NEW.booking_number := generate_booking_number();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_set_booking_number
    BEFORE INSERT ON bookings
    FOR EACH ROW
    EXECUTE FUNCTION set_booking_number();

-- Indexes
CREATE INDEX idx_bookings_event_id ON bookings(event_id);
CREATE INDEX idx_bookings_user_id ON bookings(user_id);
CREATE INDEX idx_bookings_booking_number ON bookings(booking_number);
CREATE INDEX idx_bookings_status ON bookings(booking_status, payment_status);
CREATE INDEX idx_bookings_created_at ON bookings(created_at DESC);
CREATE INDEX idx_bookings_event_user ON bookings(event_id, user_id);

-- Updated at trigger
CREATE TRIGGER update_bookings_updated_at 
    BEFORE UPDATE ON bookings
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();