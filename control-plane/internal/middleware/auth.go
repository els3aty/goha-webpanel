// Package middleware — authentication middleware.
// Validates session tokens on every protected request.
// Attaches the authenticated Principal to the request context.
package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/config"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Authenticator validates session tokens and attaches principal to context.
type Authenticator struct {
	sessions *auth.SessionManager
	pool     *pgxpool.Pool
	cfg      *config.SessionConfig
}

// NewAuthenticator creates a new Authenticator middleware.
func NewAuthenticator(sessions *auth.SessionManager, pool *pgxpool.Pool, cfg *config.SessionConfig) *Authenticator {
	return &Authenticator{sessions: sessions, pool: pool, cfg: cfg}
}

// Authenticate is middleware that validates the session cookie and sets the principal.
// Routes that call this middleware will have a principal in context if authenticated.
// The route itself must still call rbac.Require() to enforce role requirements.
func (a *Authenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := auth.GetTokenFromRequest(r, a.cfg.CookieName)
		if rawToken == "" {
			// Try Bearer token for API clients
			rawToken = extractBearerToken(r)
		}

		if rawToken == "" {
			// No credentials — continue without principal
			// The route middleware (rbac.Require) will reject if auth is needed
			next.ServeHTTP(w, r)
			return
		}

		session, err := a.sessions.Validate(r.Context(), rawToken)
		if err != nil {
			// Invalid/expired token — clear cookie and continue without principal
			a.sessions.ClearCookie(w, a.cfg)
			next.ServeHTTP(w, r)
			return
		}

		// Load user from DB to get current role (don't trust session cache for role)
		principal, err := a.loadPrincipal(r, session)
		if err != nil {
			// User deleted or suspended — revoke session
			a.sessions.Revoke(r.Context(), session.ID) //nolint:errcheck
			a.sessions.ClearCookie(w, a.cfg)
			next.ServeHTTP(w, r)
			return
		}

		// Attach principal to context
		ctx := rbac.WithPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth is middleware that returns 401 if no principal is in context.
// Use AFTER Authenticate middleware.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rbac.PrincipalFromContext(r.Context()) == nil {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loadPrincipal fetches fresh user data from DB and builds a Principal.
// We always re-query the DB to get current role/status — session cache is not trusted.
func (a *Authenticator) loadPrincipal(r *http.Request, session *auth.Session) (*rbac.Principal, error) {
	var email, role string
	var isActive, isSuspended bool
	var resellerID *uuid.UUID

	err := a.pool.QueryRow(r.Context(), `
		SELECT email, role, is_active, is_suspended, reseller_id
		FROM users
		WHERE id = $1
	`, session.UserID).Scan(&email, &role, &isActive, &isSuspended, &resellerID)
	if err != nil {
		return nil, err
	}

	if !isActive || isSuspended {
		return nil, auth.ErrSessionInvalid
	}

	return &rbac.Principal{
		UserID:      session.UserID,
		Email:       email,
		Role:        rbac.Role(role),
		SessionID:   session.ID,
		MFAVerified: session.MFAVerified,
		ResellerID:  resellerID,
		IsAPIToken:  false,
	}, nil
}

// extractBearerToken extracts a Bearer token from the Authorization header.
// Returns empty string if not present or malformed.
// NEVER logs the token value.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		token := auth[len(prefix):]
		// API tokens are 64-char hex strings
		if len(token) == 64 {
			return token
		}
	}
	return ""
}
