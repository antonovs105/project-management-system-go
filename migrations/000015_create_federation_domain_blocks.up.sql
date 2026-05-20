CREATE TABLE federation_domain_blocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT federation_domain_blocks_domain_not_blank CHECK (btrim(domain) <> ''),
    CONSTRAINT federation_domain_blocks_domain_normalized CHECK (domain = lower(domain))
);

CREATE UNIQUE INDEX idx_federation_domain_blocks_domain
    ON federation_domain_blocks(domain);

CREATE TRIGGER trg_federation_domain_blocks_updated_at BEFORE UPDATE ON federation_domain_blocks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
