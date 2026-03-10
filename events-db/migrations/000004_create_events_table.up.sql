-- migrations/000004_create_events_table.up.sql

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Basic info
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,  -- FIXED: was "descript"
    cover_image_url TEXT,
    banner_image_url TEXT,
    gallery_images JSONB,
    
    -- Date & time
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    timezone VARCHAR(50) DEFAULT 'SAST' NOT NULL,
    
    -- Location
    venue_name VARCHAR(255) NOT NULL,
    address TEXT NOT NULL,
    city VARCHAR(255) NOT NULL,
    latitude DECIMAL(10,8),
    longitude DECIMAL(11,8),
    
    -- Categorization
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    tags TEXT[],
    
    -- Pricing & capacity
    price DECIMAL(10,2) DEFAULT 0.00 NOT NULL,
    currency VARCHAR(3) DEFAULT 'ZAR' NOT NULL,
    max_capacity INTEGER,
    tickets_sold INTEGER DEFAULT 0 NOT NULL,
    allow_waitlist BOOLEAN DEFAULT TRUE,
    
    -- Status & visibility
    status event_status DEFAULT 'draft' NOT NULL,
    is_featured BOOLEAN DEFAULT FALSE,
    featured_until TIMESTAMP WITH TIME ZONE,
    published_at TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    cancellation_reason TEXT,
    
    -- Policies
    refund_policy TEXT,
    terms_and_conditions TEXT,
    
    -- Ownership
    organizer_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,  -- FIXED: was user_id
    
    -- Metrics
    views BIGINT DEFAULT 0,
    
    -- Full text search
    search_vector tsvector,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Constraints
    CONSTRAINT chk_price_positive CHECK (price >= 0),
    CONSTRAINT chk_capacity_positive CHECK (max_capacity IS NULL OR max_capacity > 0),
    CONSTRAINT chk_tickets_sold_valid CHECK (tickets_sold >= 0),
    CONSTRAINT chk_tickets_not_oversold CHECK (max_capacity IS NULL OR tickets_sold <= max_capacity),
    CONSTRAINT chk_end_after_start CHECK (end_time > start_time)
);

-- Indexes for performance
CREATE INDEX idx_events_organizer_id ON events(organizer_id);
CREATE INDEX idx_events_category_id ON events(category_id);
CREATE INDEX idx_events_status ON events(status);
CREATE INDEX idx_events_start_time ON events(start_time);
CREATE INDEX idx_events_city ON events(city);
CREATE INDEX idx_events_is_featured ON events(is_featured);
CREATE INDEX idx_events_price ON events(price);
CREATE INDEX idx_events_tags ON events USING GIN(tags);
CREATE INDEX idx_events_location ON events(latitude, longitude) WHERE latitude IS NOT NULL AND longitude IS NOT NULL;
CREATE INDEX idx_events_search_vector ON events USING GIN(search_vector);

-- Composite indexes for common queries
CREATE INDEX idx_events_status_start_time ON events(status, start_time DESC);
CREATE INDEX idx_events_category_status ON events(category_id, status);
CREATE INDEX idx_events_city_status ON events(city, status);

-- Full text search trigger
CREATE OR REPLACE FUNCTION events_search_trigger() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', coalesce(NEW.title,'')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.description,'')), 'B') ||
        setweight(to_tsvector('english', coalesce(NEW.venue_name,'')), 'C') ||
        setweight(to_tsvector('english', coalesce(NEW.city,'')), 'C') ||
        setweight(to_tsvector('english', coalesce(array_to_string(NEW.tags, ' '),'')), 'B');
    RETURN NEW;
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER events_search_vector_update 
    BEFORE INSERT OR UPDATE ON events 
    FOR EACH ROW 
    EXECUTE FUNCTION events_search_trigger();

-- Updated at trigger
CREATE TRIGGER update_events_updated_at 
    BEFORE UPDATE ON events
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();