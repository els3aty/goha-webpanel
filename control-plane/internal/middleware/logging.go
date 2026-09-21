// Package middleware provides HTTP middleware for logging with secret redaction.
//
// SECURITY: This logger NEVER logs:
//   - Passwords or password hashes
//   - Session tokens or API tokens
//   - Authorization header values
//   - Cookie values
//   - Database credentials
//   - Any field matching known sensitive patterns
package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hosting-panel/control-plane/internal/rbac"
)

// sensitiveHeaders is the list of headers whose values are always redacted.
var sensitiveHeaders = []string{
	"authorization",
	"cookie",
	"set-cookie",
	"x-api-token",
	"x-auth-token",
	"proxy-authorization",
}

// sensitiveFields is the list of JSON/form field names whose values are redacted.
var sensitiveFields = []string{
	"password", "password_confirm", "current_password",
	"token", "api_token", "secret", "api_key",
	"private_key", "totp_secret", "backup_code",
	"authorization", "cookie", "credential",
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// Logger returns a structured logging middleware.
// It logs request details but NEVER logs sensitive header values or body content.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := RequestIDFromContext(r.Context())

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			// Extract principal info safely (no secrets)
			var userID, userRole string
			if p := rbac.PrincipalFromContext(r.Context()); p != nil {
				userID = p.UserID.String()
				userRole = string(p.Role)
			}

			// Log request — notice: no body content, no cookie values, no auth values
			logger.Info("http_request",
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", sanitizePath(r.URL.Path)),
				slog.Int("status", rw.statusCode),
				slog.Int64("bytes", rw.written),
				slog.Duration("duration", duration),
				slog.String("ip", extractClientIP(r)),
				slog.String("user_id", userID),
				slog.String("user_role", userRole),
				// Deliberately NOT logging: r.URL.Query() (may contain tokens),
				// request body (may contain passwords), Cookie header
			)
		})
	}
}

// sanitizePath removes query parameters from paths to prevent token leakage.
// Some applications put tokens in query strings — we strip them.
func sanitizePath(path string) string {
	// Remove query string from path
	if idx := strings.Index(path, "?"); idx >= 0 {
		return path[:idx] + "?[redacted]"
	}
	return path
}

// extractClientIP gets the client IP safely.
func extractClientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[:idx]
	}
	return addr
}

// IsSensitiveField returns true if a field name matches a known sensitive pattern.
// Used by handlers to avoid logging sensitive request fields.
func IsSensitiveField(name string) bool {
	lower := strings.ToLower(name)
	for _, f := range sensitiveFields {
		if lower == f {
			return true
		}
	}
	return false
}

// IsSensitiveHeader returns true if a header name should have its value redacted.
func IsSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	for _, h := range sensitiveHeaders {
		if lower == h {
			return true
		}
	}
	return false
}

// SafeHeaders returns a copy of the headers map with sensitive values redacted.
// Use this when you need to log or debug headers.
func SafeHeaders(headers http.Header) map[string]string {
	safe := make(map[string]string, len(headers))
	for k, v := range headers {
		if IsSensitiveHeader(k) {
			safe[k] = "[REDACTED]"
		} else {
			safe[k] = strings.Join(v, ", ")
		}
	}
	return safe
}
