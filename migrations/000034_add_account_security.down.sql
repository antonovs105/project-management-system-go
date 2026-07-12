DROP TABLE IF EXISTS email_outbox;
DROP TABLE IF EXISTS auth_events;
DROP TABLE IF EXISTS user_mfa_credentials;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS account_tokens;

ALTER TABLE users
    DROP COLUMN IF EXISTS email_verified;
