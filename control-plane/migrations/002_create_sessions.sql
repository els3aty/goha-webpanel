-- Migration 002: Create sessions table
-- Phase 2: Control Plane + Auth + RBAC

BEGIN;

CREATE TABLE IF NOT EXISTS sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- SHA-256 hash of the raw token — raw token is NEVER stored
    token_hash   TEXT NOT NULL,
    ip_address   INET NOT NULL,
    user_agent   TEXT NOT NULL DEFAULT '',
    -- MFA state for this session
    mfa_verified BOOLEAN NOT NULL DEFAULT FALSE,
    -- Timestamps
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ,
    -- Constraints
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash)
);

-- Fast token lookup (every request)
CREATE INDEX idx_sessions_token_hash ON sessions (token_hash)
    WHERE revoked_at IS NULL;

-- Cleanup: find expired/revoked sessions by user
CREATE INDEX idx_sessions_user_id ON sessions (user_id, created_at DESC);

-- Scheduled cleanup index
CREATE INDEX idx_sessions_expires ON sessions (expires_at)
    WHERE revoked_at IS NULL;

COMMIT;
