-- 006_create_hosting.sql

-- Hosting Users (Linux accounts)
CREATE TABLE hosting_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    username VARCHAR(32) NOT NULL, -- Linux username (e.g. 'user123')
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'locked', 'deleting', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    CONSTRAINT hosting_users_username_node_unique UNIQUE (node_id, username)
);
CREATE INDEX idx_hosting_users_customer ON hosting_users(customer_id);

-- Trigger for hosting_users
CREATE TRIGGER hosting_users_updated_at
    BEFORE UPDATE ON hosting_users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Virtual Hosts (Websites)
CREATE TABLE virtual_hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    domain VARCHAR(253) NOT NULL UNIQUE,
    document_root VARCHAR(255) NOT NULL,
    php_version VARCHAR(10), -- e.g., '8.1', '8.2'
    ssl_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleting', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
CREATE INDEX idx_virtual_hosts_user ON virtual_hosts(hosting_user_id);

-- Trigger for virtual_hosts
CREATE TRIGGER virtual_hosts_updated_at
    BEFORE UPDATE ON virtual_hosts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
