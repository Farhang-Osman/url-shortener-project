-- +goose Up
CREATE TABLE analytics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(50) NOT NULL, -- 'url_created' or 'url_clicked'
    short_code VARCHAR(20),
    long_url TEXT,
    user_id UUID,
    user_agent TEXT,
    referer TEXT,
    ip_address INET,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for better query performance
CREATE INDEX idx_analytics_event_type ON analytics(event_type);
CREATE INDEX idx_analytics_short_code ON analytics(short_code);
CREATE INDEX idx_analytics_timestamp ON analytics(timestamp);

-- +goose Down
DROP TABLE analytics;
