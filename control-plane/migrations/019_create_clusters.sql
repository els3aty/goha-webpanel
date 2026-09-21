-- 019_create_clusters.sql

CREATE TABLE clusters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE TRIGGER clusters_updated_at
    BEFORE UPDATE ON clusters
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add role and cluster_id to nodes
ALTER TABLE nodes 
ADD COLUMN role VARCHAR(50) NOT NULL DEFAULT 'compute',
ADD COLUMN cluster_id UUID REFERENCES clusters(id);
