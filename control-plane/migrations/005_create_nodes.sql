-- 005_create_nodes.sql

-- Nodes table stores information about physical servers (Agents)
CREATE TABLE nodes (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    address VARCHAR(255) NOT NULL, -- The IP:Port of the agent (e.g., 10.0.0.5:9090)
    signing_key_encrypted TEXT NOT NULL, -- AES-256-GCM encrypted HMAC signing key
    status VARCHAR(50) NOT NULL DEFAULT 'offline', -- 'online', 'offline', 'maintenance'
    version VARCHAR(50), -- The agent protocol version
    last_seen_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

-- Index for quick lookups by status
CREATE INDEX idx_nodes_status ON nodes(status);

-- Ensure node names are unique
CREATE UNIQUE INDEX idx_nodes_name ON nodes(name);
