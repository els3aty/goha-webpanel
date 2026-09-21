-- Migration 003: Create API tokens table
-- Phase 2: Control Plane + Auth + RBAC

BEGIN;

CREATE TABLE IF NOT EXISTS api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    -- SHA-256 hash of the raw token — raw token shown only once at creation
    token_hash   TEXT NOT NULL,
    -- JSON array of scope strings e.g. ["domains:read","domains:write"]
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    -- Optional IP allowlist — empty means unrestricted
    ip_allowlist INET[] NOT NULL DEFAULT '{}',
    -- Timestamps
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    -- Constraints
    CONSTRAINT api_tokens_hash_unique UNIQUE (token_hash)
);

CREATE INDEX idx_api_tokens_user_id   ON api_tokens (user_id);
CREATE INDEX idx_api_tokens_hash      ON api_tokens (token_hash) WHERE revoked_at IS NULL;

COMMIT;
