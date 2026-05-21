ALTER TABLE users
ADD COLUMN token_version INTEGER NOT NULL DEFAULT 1 CHECK (token_version > 0);
