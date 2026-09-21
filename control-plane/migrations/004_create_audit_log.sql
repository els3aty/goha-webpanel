-- Migration 004: Create audit_log table
-- Phase 2: Control Plane + Auth + RBAC
-- IMPORTANT: This table is APPEND-ONLY.
-- The application DB user must NOT have UPDATE or DELETE on this table.

BEGIN;

CREATE TABLE IF NOT EXISTS audit_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Actor info (NULL if system-generated)
    actor_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_role    TEXT,
    actor_email   TEXT, -- Denormalized for audit trail if user is deleted
    -- What happened
    action        TEXT NOT NULL,   -- e.g. "auth.login", "domain.create"
    resource_type TEXT,            -- e.g. "domain", "user", "session"
    resource_id   UUID,
    -- Context
    ip_address    INET,
    user_agent    TEXT,
    request_id    TEXT,
    session_id    UUID,
    -- Result
    result        TEXT NOT NULL CHECK (result IN ('success','failure','error')),
    -- Safe metadata — MUST NOT contain passwords, tokens, or secrets
    -- Logged by redaction middleware before storage
    metadata      JSONB NOT NULL DEFAULT '{}',
    -- Timestamp (immutable)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
    -- NO updated_at — this table is append-only
);

-- Query patterns
CREATE INDEX idx_audit_actor      ON audit_log (actor_id, created_at DESC)
    WHERE actor_id IS NOT NULL;
CREATE INDEX idx_audit_action     ON audit_log (action, created_at DESC);
CREATE INDEX idx_audit_resource   ON audit_log (resource_type, resource_id)
    WHERE resource_id IS NOT NULL;
CREATE INDEX idx_audit_request_id ON audit_log (request_id)
    WHERE request_id IS NOT NULL;
CREATE INDEX idx_audit_created    ON audit_log (created_at DESC);

-- SECURITY: Revoke UPDATE and DELETE from application role
-- Run as superuser after migration:
-- REVOKE UPDATE, DELETE ON audit_log FROM hosting_panel_app;
-- GRANT INSERT, SELECT ON audit_log TO hosting_panel_app;

COMMIT;
