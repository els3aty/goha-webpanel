-- 013_create_mail.sql

CREATE TABLE mailboxes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    address VARCHAR(255) NOT NULL UNIQUE, -- e.g. info@domain.com
    password_hash VARCHAR(255) NOT NULL,
    quota_mb INTEGER NOT NULL DEFAULT 1024,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleting', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX idx_mailboxes_user ON mailboxes(hosting_user_id);
CREATE INDEX idx_mailboxes_domain ON mailboxes(domain);

CREATE TRIGGER mailboxes_updated_at
    BEFORE UPDATE ON mailboxes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE mail_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    source VARCHAR(255) NOT NULL UNIQUE, -- e.g. alias@domain.com
    destination VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX idx_mail_aliases_user ON mail_aliases(hosting_user_id);

CREATE TRIGGER mail_aliases_updated_at
    BEFORE UPDATE ON mail_aliases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
