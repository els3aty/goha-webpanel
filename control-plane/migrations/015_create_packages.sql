-- 015_create_packages.sql

CREATE TABLE hosting_packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    disk_quota_mb INTEGER NOT NULL,
    max_domains INTEGER NOT NULL,
    max_databases INTEGER NOT NULL,
    max_mailboxes INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE TRIGGER hosting_packages_updated_at
    BEFORE UPDATE ON hosting_packages
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add package_id to hosting_users
ALTER TABLE hosting_users ADD COLUMN package_id UUID REFERENCES hosting_packages(id);

-- Optional: Create a default package if one doesn't exist
INSERT INTO hosting_packages (id, name, disk_quota_mb, max_domains, max_databases, max_mailboxes)
VALUES (gen_random_uuid(), 'Default Package', 5120, 5, 5, 10)
ON CONFLICT (name) DO NOTHING;

-- Update existing users to use the default package (assuming they didn't have one)
UPDATE hosting_users SET package_id = (SELECT id FROM hosting_packages WHERE name = 'Default Package') WHERE package_id IS NULL;

-- Now make it NOT NULL
ALTER TABLE hosting_users ALTER COLUMN package_id SET NOT NULL;
