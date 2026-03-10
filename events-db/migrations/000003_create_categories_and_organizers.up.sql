-- migrations/000003_create_categories_and_organizers.up.sql

-- Categories table
CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    image_url TEXT,
    display_order INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_categories_slug ON categories(slug);
CREATE INDEX idx_categories_active ON categories(is_active);

-- Insert default categories
INSERT INTO categories (name, slug, description, display_order) VALUES
    ('Music', 'music', 'Concerts, festivals, and live performances', 1),
    ('Sports', 'sports', 'Sporting events and competitions', 2),
    ('Arts & Theatre', 'arts-theatre', 'Theater, dance, and performing arts', 3),
    ('Food & Drink', 'food-drink', 'Food festivals, tastings, and culinary events', 4),
    ('Business', 'business', 'Conferences, networking, and professional development', 5),
    ('Community', 'community', 'Local gatherings and community events', 6),
    ('Technology', 'technology', 'Tech meetups, hackathons, and innovation', 7),
    ('Health & Wellness', 'health-wellness', 'Fitness, yoga, and wellness activities', 8),
    ('Education', 'education', 'Workshops, classes, and learning experiences', 9),
    ('Other', 'other', 'Other events and activities', 10);

-- Organizers table
CREATE TABLE organizers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    business_name VARCHAR(255) NOT NULL,
    business_email VARCHAR(255) NOT NULL,
    business_phone VARCHAR(20) NOT NULL,
    tax_id VARCHAR(50),
    website_url TEXT,
    description TEXT,
    logo_url TEXT,
    verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMP WITH TIME ZONE,
    verified_by UUID REFERENCES users(id),
    commission_rate DECIMAL(5,2) DEFAULT 10.00,
    stripe_account_id VARCHAR(255),
    payout_enabled BOOLEAN DEFAULT FALSE,
    bank_account_last_4 VARCHAR(4),
    status organizer_status DEFAULT 'pending' NOT NULL,
    rejection_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT unique_user_organizer UNIQUE(user_id),
    CONSTRAINT valid_commission_rate CHECK (commission_rate >= 0 AND commission_rate <= 100)
);

CREATE INDEX idx_organizers_user_id ON organizers(user_id);
CREATE INDEX idx_organizers_verified ON organizers(verified);
CREATE INDEX idx_organizers_status ON organizers(status);
CREATE INDEX idx_organizers_stripe_account ON organizers(stripe_account_id);

CREATE TRIGGER update_organizers_updated_at 
    BEFORE UPDATE ON organizers
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();