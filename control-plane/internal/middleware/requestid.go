// Package middleware — request ID middleware.
// Every request gets a unique ID for tracing across logs and audit entries.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const requestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// RequestID adds a unique request ID to every incoming request.
// The ID is set in the response header and stored in context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use existing request ID if provided by trusted upstream proxy
		reqID := r.Header.Get(requestIDHeader)
		if reqID == "" || len(reqID) > 64 {
			reqID = generateRequestID()
		}

		// Store in context
		ctx := context.WithValue(r.Context(), requestIDKey{}, reqID)

		// Echo back in response header for client-side correlation
		w.Header().Set(requestIDHeader, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext retrieves the request ID from context.
// Returns empty string if not set.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// generateRequestID creates a cryptographically random 16-byte hex request ID.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
