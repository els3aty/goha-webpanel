// Package db — migration runner.
// Migrations are embedded from the migrations/ directory and run in order.
// Each migration runs in a transaction. Failed migrations abort.
package db

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration represents a single SQL migration file.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrator runs database migrations in order.
type Migrator struct {
	pool *pgxpool.Pool
}

// NewMigrator creates a new Migrator with the given connection pool.
func NewMigrator(pool *pgxpool.Pool) *Migrator {
	return &Migrator{pool: pool}
}

// Run applies all pending migrations in version order.
// Each migration is wrapped in a transaction — all-or-nothing.
func (m *Migrator) Run(ctx context.Context, migrations []Migration) error {
	if err := m.ensureSchema(ctx); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to load applied migrations: %w", err)
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	for _, mg := range migrations {
		if applied[mg.Version] {
			continue // already applied
		}

		if err := m.apply(ctx, mg); err != nil {
			return fmt.Errorf("migration %03d_%s failed: %w", mg.Version, mg.Name, err)
		}
	}

	return nil
}

// ensureSchema creates the schema_migrations tracking table if it doesn't exist.
func (m *Migrator) ensureSchema(ctx context.Context) error {
	_, err := m.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			name        TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

// appliedVersions returns a set of already-applied migration versions.
func (m *Migrator) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := m.pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// apply runs a single migration inside a transaction.
func (m *Migrator) apply(ctx context.Context, mg Migration) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Run the migration SQL
	if _, err := tx.Exec(ctx, mg.SQL); err != nil {
		return fmt.Errorf("SQL error: %w", sanitizeSQLError(err))
	}

	// Record it as applied
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		mg.Version, mg.Name,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// sanitizeSQLError prevents SQL error messages from leaking schema details.
func sanitizeSQLError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	// Truncate very long errors that might contain data
	if len(msg) > 500 {
		return fmt.Errorf("SQL error (truncated): %s", msg[:200])
	}
	return err
}

// AllMigrations returns all migrations as a slice.
// In production, these would be embedded via go:embed.
// For Phase 2, they are defined inline and loaded from the migrations/ directory.
func AllMigrations() []Migration {
	return []Migration{
		{Version: 1, Name: "create_users", SQL: migration001},
		{Version: 2, Name: "create_sessions", SQL: migration002},
		{Version: 3, Name: "create_api_tokens", SQL: migration003},
		{Version: 4, Name: "create_audit_log", SQL: migration004},
		{Version: 5, Name: "create_nodes", SQL: migration005},
		{Version: 6, Name: "create_hosting", SQL: migration006},
		{Version: 7, Name: "create_databases", SQL: migration007},
		{Version: 8, Name: "create_dns", SQL: migration008},
		{Version: 9, Name: "create_backups", SQL: migration009},
		{Version: 10, Name: "create_nodejs", SQL: migration010},
		{Version: 11, Name: "create_whmcs_tokens", SQL: migration011},
		{Version: 13, Name: "create_mail", SQL: migration013},
		{Version: 15, Name: "create_packages", SQL: migration015},
		{Version: 17, Name: "create_runtimes", SQL: migration017},
		{Version: 19, Name: "create_clusters", SQL: migration019},
	}
}

// Migration SQL content (embedded for simplicity in Phase 2).
// Phase 19 (installer) will use go:embed or file-based loading.
const migration001 = `
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    TEXT NOT NULL,
    email       TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role        TEXT NOT NULL CHECK (role IN ('super_admin','admin','reseller','customer','api_integration')),
    reseller_id UUID REFERENCES users(id) ON DELETE SET NULL,
    mfa_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    totp_secret_encrypted TEXT,
    mfa_backup_codes_hash TEXT[],
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    is_suspended BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at   TIMESTAMPTZ,
    password_changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_username_unique UNIQUE (username),
    CONSTRAINT users_email_unique    UNIQUE (email)
);
CREATE INDEX IF NOT EXISTS idx_users_email    ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_username ON users (username);
CREATE INDEX IF NOT EXISTS idx_users_role     ON users (role);
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS users_updated_at ON users;
CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration002 = `
CREATE TABLE IF NOT EXISTS sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL,
    ip_address   INET NOT NULL,
    user_agent   TEXT NOT NULL DEFAULT '',
    mfa_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ,
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash)
);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions (token_hash) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_user_id    ON sessions (user_id, created_at DESC);
`

const migration003 = `
CREATE TABLE IF NOT EXISTS api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL,
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    ip_allowlist INET[] NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    CONSTRAINT api_tokens_hash_unique UNIQUE (token_hash)
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash    ON api_tokens (token_hash) WHERE revoked_at IS NULL;
`

const migration004 = `
CREATE TABLE IF NOT EXISTS audit_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_role    TEXT,
    actor_email   TEXT,
    action        TEXT NOT NULL,
    resource_type TEXT,
    resource_id   UUID,
    ip_address    INET,
    user_agent    TEXT,
    request_id    TEXT,
    session_id    UUID,
    result        TEXT NOT NULL CHECK (result IN ('success','failure','error')),
    metadata      JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_actor    ON audit_log (actor_id, created_at DESC) WHERE actor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_action   ON audit_log (action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_created  ON audit_log (created_at DESC);
`

const migration005 = `
CREATE TABLE IF NOT EXISTS nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    signing_key_encrypted TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('online', 'offline', 'maintenance')),
    version TEXT,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT nodes_name_unique UNIQUE (name)
);
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes (status);
DROP TRIGGER IF EXISTS nodes_updated_at ON nodes;
CREATE TRIGGER nodes_updated_at
    BEFORE UPDATE ON nodes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration006 = `
CREATE TABLE IF NOT EXISTS hosting_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    username VARCHAR(32) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'locked', 'deleting', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT hosting_users_username_node_unique UNIQUE (node_id, username)
);
CREATE INDEX IF NOT EXISTS idx_hosting_users_customer ON hosting_users(customer_id);
DROP TRIGGER IF EXISTS hosting_users_updated_at ON hosting_users;
CREATE TRIGGER hosting_users_updated_at
    BEFORE UPDATE ON hosting_users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE IF NOT EXISTS virtual_hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    domain VARCHAR(253) NOT NULL UNIQUE,
    document_root VARCHAR(255) NOT NULL,
    php_version VARCHAR(10),
    ssl_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleting', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_virtual_hosts_user ON virtual_hosts(hosting_user_id);
DROP TRIGGER IF EXISTS virtual_hosts_updated_at ON virtual_hosts;
CREATE TRIGGER virtual_hosts_updated_at
    BEFORE UPDATE ON virtual_hosts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration007 = `
CREATE TABLE IF NOT EXISTS databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    db_name VARCHAR(64) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleting', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_databases_user ON databases(hosting_user_id);
DROP TRIGGER IF EXISTS databases_updated_at ON databases;
CREATE TRIGGER databases_updated_at
    BEFORE UPDATE ON databases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE IF NOT EXISTS database_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    db_username VARCHAR(32) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleting', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_database_users_hosting_user ON database_users(hosting_user_id);
DROP TRIGGER IF EXISTS database_users_updated_at ON database_users;
CREATE TRIGGER database_users_updated_at
    BEFORE UPDATE ON database_users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE IF NOT EXISTS database_grants (
    database_id UUID NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
    database_user_id UUID NOT NULL REFERENCES database_users(id) ON DELETE CASCADE,
    privileges TEXT NOT NULL DEFAULT 'ALL PRIVILEGES',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (database_id, database_user_id)
);
`

const migration008 = `
CREATE TABLE IF NOT EXISTS dns_zones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    domain VARCHAR(253) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleting', 'failed')),
    serial BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dns_zones_user ON dns_zones(hosting_user_id);
DROP TRIGGER IF EXISTS dns_zones_updated_at ON dns_zones;
CREATE TRIGGER dns_zones_updated_at
    BEFORE UPDATE ON dns_zones
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE IF NOT EXISTS dns_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone_id UUID NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    name VARCHAR(253) NOT NULL,
    type VARCHAR(10) NOT NULL CHECK (type IN ('A', 'AAAA', 'CNAME', 'MX', 'TXT', 'SRV', 'NS')),
    content TEXT NOT NULL,
    ttl INTEGER NOT NULL DEFAULT 3600,
    priority INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dns_records_zone ON dns_records(zone_id);
DROP TRIGGER IF EXISTS dns_records_updated_at ON dns_records;
CREATE TRIGGER dns_records_updated_at
    BEFORE UPDATE ON dns_records
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration009 = `
CREATE TABLE IF NOT EXISTS backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    backup_type VARCHAR(20) NOT NULL CHECK (backup_type IN ('files', 'database', 'full')),
    file_path VARCHAR(512) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_backups_user ON backups(hosting_user_id);
DROP TRIGGER IF EXISTS backups_updated_at ON backups;
CREATE TRIGGER backups_updated_at
    BEFORE UPDATE ON backups
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration010 = `
CREATE TABLE IF NOT EXISTS nodejs_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    app_name VARCHAR(100) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    app_path VARCHAR(512) NOT NULL,
    startup_file VARCHAR(255) NOT NULL,
    node_version VARCHAR(20) NOT NULL DEFAULT '20',
    port INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'stopped' CHECK (status IN ('stopped', 'running', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(hosting_user_id, app_name),
    UNIQUE(domain)
);
CREATE INDEX IF NOT EXISTS idx_nodejs_apps_user ON nodejs_apps(hosting_user_id);
CREATE INDEX IF NOT EXISTS idx_nodejs_apps_domain ON nodejs_apps(domain);

DROP TRIGGER IF EXISTS nodejs_apps_updated_at ON nodejs_apps;
CREATE TRIGGER nodejs_apps_updated_at
    BEFORE UPDATE ON nodejs_apps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration011 = `
CREATE TABLE IF NOT EXISTS whmcs_api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    allowed_ips JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(token_hash)
);

DROP TRIGGER IF EXISTS whmcs_api_tokens_updated_at ON whmcs_api_tokens;
CREATE TRIGGER whmcs_api_tokens_updated_at
    BEFORE UPDATE ON whmcs_api_tokens
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration013 = `
CREATE TABLE IF NOT EXISTS mailboxes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    address VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    quota_mb INTEGER NOT NULL DEFAULT 1024,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleting', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mailboxes_user ON mailboxes(hosting_user_id);
CREATE INDEX IF NOT EXISTS idx_mailboxes_domain ON mailboxes(domain);

DROP TRIGGER IF EXISTS mailboxes_updated_at ON mailboxes;
CREATE TRIGGER mailboxes_updated_at
    BEFORE UPDATE ON mailboxes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE IF NOT EXISTS mail_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hosting_user_id UUID NOT NULL REFERENCES hosting_users(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    source VARCHAR(255) NOT NULL UNIQUE,
    destination VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mail_aliases_user ON mail_aliases(hosting_user_id);

DROP TRIGGER IF EXISTS mail_aliases_updated_at ON mail_aliases;
CREATE TRIGGER mail_aliases_updated_at
    BEFORE UPDATE ON mail_aliases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
`

const migration015 = `
CREATE TABLE IF NOT EXISTS hosting_packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    disk_quota_mb INTEGER NOT NULL,
    max_domains INTEGER NOT NULL,
    max_databases INTEGER NOT NULL,
    max_mailboxes INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DROP TRIGGER IF EXISTS hosting_packages_updated_at ON hosting_packages;
CREATE TRIGGER hosting_packages_updated_at
    BEFORE UPDATE ON hosting_packages
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE hosting_users ADD COLUMN IF NOT EXISTS package_id UUID REFERENCES hosting_packages(id);

INSERT INTO hosting_packages (name, disk_quota_mb, max_domains, max_databases, max_mailboxes)
VALUES ('Default Package', 5120, 5, 5, 10)
ON CONFLICT (name) DO NOTHING;

UPDATE hosting_users SET package_id = (SELECT id FROM hosting_packages WHERE name = 'Default Package' LIMIT 1) WHERE package_id IS NULL;

ALTER TABLE hosting_users ALTER COLUMN package_id SET NOT NULL;
`

const migration017 = `
ALTER TABLE virtual_hosts 
ADD COLUMN IF NOT EXISTS runtime_type VARCHAR(50) NOT NULL DEFAULT 'php',
ADD COLUMN IF NOT EXISTS runtime_port INTEGER;

ALTER TABLE virtual_hosts DROP CONSTRAINT IF EXISTS chk_runtime_port;
ALTER TABLE virtual_hosts
ADD CONSTRAINT chk_runtime_port 
CHECK (runtime_type = 'php' OR runtime_port IS NOT NULL);
`

const migration019 = `
CREATE TABLE IF NOT EXISTS clusters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DROP TRIGGER IF EXISTS clusters_updated_at ON clusters;
CREATE TRIGGER clusters_updated_at
    BEFORE UPDATE ON clusters
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE nodes 
ADD COLUMN IF NOT EXISTS role VARCHAR(50) NOT NULL DEFAULT 'compute',
ADD COLUMN IF NOT EXISTS cluster_id UUID REFERENCES clusters(id);
`

// stripSensitive removes sensitive patterns from strings (used in error handling).
func stripSensitive(s string) string {
	patterns := []string{"password=", "PASSWORD=", "token=", "TOKEN="}
	for _, p := range patterns {
		if idx := strings.Index(s, p); idx >= 0 {
			s = s[:idx] + "[REDACTED]"
		}
	}
	return s
}
