-- 011_create_whmcs_tokens.sql

CREATE TABLE whmcs_api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    allowed_ips JSONB, -- Array of allowed IP addresses, or null/empty for any IP (not recommended)
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    last_used_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    UNIQUE(token_hash)
);

CREATE TRIGGER whmcs_api_tokens_updated_at
    BEFORE UPDATE ON whmcs_api_tokens
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
