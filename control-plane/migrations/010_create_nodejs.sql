-- 010_create_nodejs.sql

CREATE TABLE nodejs_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    app_name VARCHAR(100) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    app_path VARCHAR(512) NOT NULL,
    startup_file VARCHAR(255) NOT NULL,
    node_version VARCHAR(20) NOT NULL DEFAULT '20',
    port INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'stopped' CHECK (status IN ('stopped', 'running', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    UNIQUE(hosting_user_id, app_name),
    UNIQUE(domain)
);

CREATE INDEX idx_nodejs_apps_user ON nodejs_apps(hosting_user_id);
CREATE INDEX idx_nodejs_apps_domain ON nodejs_apps(domain);

CREATE TRIGGER nodejs_apps_updated_at
    BEFORE UPDATE ON nodejs_apps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
