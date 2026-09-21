-- 007_create_databases.sql

-- Databases
CREATE TABLE databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    db_name VARCHAR(64) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleting', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
CREATE INDEX idx_databases_user ON databases(hosting_user_id);

CREATE TRIGGER databases_updated_at
    BEFORE UPDATE ON databases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Database Users
CREATE TABLE database_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    db_username VARCHAR(32) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleting', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
CREATE INDEX idx_database_users_hosting_user ON database_users(hosting_user_id);

CREATE TRIGGER database_users_updated_at
    BEFORE UPDATE ON database_users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Database User Grants (Mapping table)
CREATE TABLE database_grants (
    database_id UUID NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
    database_user_id UUID NOT NULL REFERENCES database_users(id) ON DELETE CASCADE,
    privileges TEXT NOT NULL DEFAULT 'ALL PRIVILEGES',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    PRIMARY KEY (database_id, database_user_id)
);
