// Package audit provides append-only audit logging for all privileged operations.
//
// SECURITY:
//   - Audit log is append-only (INSERT only — no UPDATE, no DELETE)
//   - Secrets must NEVER appear in metadata — caller is responsible for redaction
//   - Every privileged action must produce an audit entry
//   - Failures are logged with result="failure" or result="error"
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/middleware"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Result represents the outcome of an audited operation.
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	ResultError   Result = "error"
)

// Event represents an audit log entry.
type Event struct {
	ActorID      *uuid.UUID
	ActorRole    string
	ActorEmail   string
	Action       string // e.g. "auth.login", "domain.create", "user.delete"
	ResourceType string // e.g. "domain", "user", "session"
	ResourceID   *uuid.UUID
	IPAddress    string
	UserAgent    string
	RequestID    string
	SessionID    *uuid.UUID
	Result       Result
	// Metadata contains SAFE metadata — NEVER include secrets, passwords, or tokens.
	// Caller must redact sensitive fields before passing.
	Metadata map[string]any
}

// Logger writes audit events to PostgreSQL and optionally to structured log output.
type Logger struct {
	pool    *pgxpool.Pool
	slogger *slog.Logger
}

// New creates a new audit Logger.
func New(pool *pgxpool.Pool, slogger *slog.Logger) *Logger {
	return &Logger{pool: pool, slogger: slogger}
}

// Log records an audit event.
// This function must not fail silently — if DB write fails, it logs to stderr.
// Errors in audit logging should never be swallowed.
func (l *Logger) Log(ctx context.Context, event Event) {
	// Validate that metadata doesn't contain obvious secrets
	event.Metadata = sanitizeMetadata(event.Metadata)

	metadataJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	_, dbErr := l.pool.Exec(ctx, `
		INSERT INTO audit_log (
			actor_id, actor_role, actor_email,
			action, resource_type, resource_id,
			ip_address, user_agent, request_id, session_id,
			result, metadata
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7::inet, $8, $9, $10,
			$11, $12
		)
	`,
		event.ActorID, event.ActorRole, event.ActorEmail,
		event.Action, event.ResourceType, event.ResourceID,
		event.IPAddress, event.UserAgent, event.RequestID, event.SessionID,
		string(event.Result), string(metadataJSON),
	)

	if dbErr != nil {
		// Audit log write failed — this is serious, log to stderr
		l.slogger.Error("AUDIT LOG WRITE FAILED",
			slog.String("action", event.Action),
			slog.String("result", string(event.Result)),
			slog.String("error", dbErr.Error()),
		)
	}

	// Also emit to structured log for real-time monitoring
	l.slogger.Info("audit",
		slog.String("action", event.Action),
		slog.String("result", string(event.Result)),
		slog.String("actor_role", event.ActorRole),
		slog.String("resource_type", event.ResourceType),
		slog.String("request_id", event.RequestID),
		slog.String("ip", event.IPAddress),
	)
}

// LogFromContext creates and logs an audit event using principal from request context.
// This is the preferred way to log from HTTP handlers.
func (l *Logger) LogFromContext(
	ctx context.Context,
	action string,
	resourceType string,
	resourceID *uuid.UUID,
	result Result,
	metadata map[string]any,
) {
	event := Event{
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       result,
		Metadata:     metadata,
		RequestID:    middleware.RequestIDFromContext(ctx),
	}

	// Extract principal from context
	if p := rbac.PrincipalFromContext(ctx); p != nil {
		event.ActorID = &p.UserID
		event.ActorRole = string(p.Role)
		event.ActorEmail = p.Email
		sid := p.SessionID
		event.SessionID = &sid
	}

	l.Log(ctx, event)
}

// sanitizeMetadata removes any keys that look like they might contain secrets.
// This is a defense-in-depth check — callers should already redact sensitive data.
func sanitizeMetadata(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	sensitiveKeys := []string{
		"password", "token", "secret", "key", "credential",
		"authorization", "cookie", "api_key", "private_key",
		"totp_secret", "backup_code", "hash",
	}
	cleaned := make(map[string]any, len(m))
	for k, v := range m {
		isSensitive := false
		kLower := toLower(k)
		for _, s := range sensitiveKeys {
			if kLower == s || contains(kLower, s) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			cleaned[k] = "[REDACTED]"
		} else {
			cleaned[k] = v
		}
	}
	return cleaned
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsAt(s, substr)))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
