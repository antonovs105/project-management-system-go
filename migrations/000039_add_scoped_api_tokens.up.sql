CREATE TABLE api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    token_prefix TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    scopes TEXT[] NOT NULL CHECK (
        cardinality(scopes) > 0
        AND scopes <@ ARRAY['projects:read', 'projects:write', 'account:read', 'account:write', 'tokens:manage', 'admin']::TEXT[]
    ),
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_tokens_user_active
    ON api_tokens (user_id, created_at DESC)
    WHERE revoked_at IS NULL;
