-- +goose Up
CREATE TABLE urls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_code VARCHAR(20) UNIQUE NOT NULL,
    long_url TEXT NOT NULL,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    click_count BIGINT DEFAULT 0,
    last_accessed TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_urls_user_id ON urls(user_id);
CREATE INDEX idx_urls_expires_at ON urls(expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_urls_expires_at;
DROP INDEX IF EXISTS idx_urls_user_id;
DROP TABLE urls;