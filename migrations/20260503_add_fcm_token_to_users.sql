-- Add fcm_token column to users table
ALTER TABLE users ADD COLUMN fcm_token TEXT;

-- Index for faster lookup if needed
CREATE INDEX idx_users_fcm_token ON users(fcm_token) WHERE fcm_token IS NOT NULL;
