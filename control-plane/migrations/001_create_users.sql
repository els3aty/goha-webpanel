-- Migration 001: Create users table
-- Phase 2: Control Plane + Auth + RBAC
-- Run: forward only in production; see 001_create_users.down.sql for rollback

BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    TEXT NOT NULL,
    email       TEXT NOT NULL,
    -- Argon2id hash — NEVER store plaintext passwords
    password_hash TEXT NOT NULL,
    role        TEXT NOT NULL CHECK (role IN ('super_admin','admin','reseller','customer','api_integration')),
    reseller_id UUID REFERENCES users(id) ON DELETE SET NULL,
    -- MFA
    mfa_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    totp_secret_encrypted TEXT,   -- AES-256-GCM encrypted TOTP seed
    mfa_backup_codes_hash TEXT[], -- bcrypt hashes of backup codes
    -- Status
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    is_suspended BOOLEAN NOT NULL DEFAULT FALSE,
    -- Timestamps
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at   TIMESTAMPTZ,
    password_changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Constraints
    CONSTRAINT users_username_unique UNIQUE (username),
    CONSTRAINT users_email_unique    UNIQUE (email)
);

-- Index for login lookups
CREATE INDEX idx_users_email    ON users (email);
CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_role     ON users (role);
CREATE INDEX idx_users_reseller ON users (reseller_id) WHERE reseller_id IS NOT NULL;

-- Trigger: auto-update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMIT;
