// Package auth — session management.
// Sessions are stored server-side in PostgreSQL.
// Only the SHA-256 hash of the raw token is stored — the raw token is never persisted.
//
// SECURITY:
//   - Raw token: 256-bit cryptographically random, shown to client once in cookie
//   - Stored: SHA-256(raw_token) — one-way, useless if DB is compromised
//   - Cookie: HttpOnly, Secure, SameSite=Strict
//   - Rotation: new token issued after each privilege change
//   - Revocation: immediate, single session or all sessions
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// tokenBytes is the number of random bytes in a session token (256 bits).
	tokenBytes = 32
)

// Session represents an active user session.
type Session struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	TokenHash   string // stored in DB — never the raw token
	IPAddress   string
	UserAgent   string
	MFAVerified bool
	CreatedAt   time.Time
	ExpiresAt   time.Time
	LastSeenAt  time.Time
	RevokedAt   *time.Time
}

// SessionManager handles session creation, validation, and revocation.
type SessionManager struct {
	pool   *pgxpool.Pool
	cfg    *config.SessionConfig
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(pool *pgxpool.Pool, cfg *config.SessionConfig) *SessionManager {
	return &SessionManager{pool: pool, cfg: cfg}
}

// Create creates a new session for the given user and returns the raw token.
// The raw token is returned ONCE and must be set in the session cookie.
// It is NEVER stored in the database.
func (sm *SessionManager) Create(
	ctx context.Context,
	userID uuid.UUID,
	r *http.Request,
	mfaVerified bool,
) (rawToken string, session *Session, err error) {
	// Generate 256-bit cryptographically random token
	tokenBytes := make([]byte, tokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate session token: %w", err)
	}
	rawToken = hex.EncodeToString(tokenBytes)

	// Hash the token for storage
	tokenHash := hashToken(rawToken)

	expiresAt := time.Now().Add(sm.cfg.MaxAge)
	ip := extractIP(r)
	ua := r.UserAgent()
	if len(ua) > 500 {
		ua = ua[:500] // truncate overly long user agents
	}

	session = &Session{
		ID:          uuid.New(),
		UserID:      userID,
		TokenHash:   tokenHash,
		IPAddress:   ip,
		UserAgent:   ua,
		MFAVerified: mfaVerified,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		LastSeenAt:  time.Now(),
	}

	_, err = sm.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, ip_address, user_agent, mfa_verified, expires_at)
		VALUES ($1, $2, $3, $4::inet, $5, $6, $7)
	`, session.ID, session.UserID, session.TokenHash,
		session.IPAddress, session.UserAgent, session.MFAVerified, session.ExpiresAt)
	if err != nil {
		return "", nil, fmt.Errorf("failed to store session: %w", err)
	}

	return rawToken, session, nil
}

// Validate looks up a session by raw token.
// Returns the session if valid, or an error if expired/revoked/not found.
// Updates last_seen_at on each valid lookup.
func (sm *SessionManager) Validate(ctx context.Context, rawToken string) (*Session, error) {
	tokenHash := hashToken(rawToken)

	var s Session
	var revokedAt *time.Time

	err := sm.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, ip_address::text, user_agent,
		       mfa_verified, created_at, expires_at, last_seen_at, revoked_at
		FROM sessions
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`, tokenHash).Scan(
		&s.ID, &s.UserID, &s.TokenHash, &s.IPAddress, &s.UserAgent,
		&s.MFAVerified, &s.CreatedAt, &s.ExpiresAt, &s.LastSeenAt, &revokedAt,
	)
	if err != nil {
		return nil, ErrSessionInvalid
	}

	s.RevokedAt = revokedAt

	// Update last_seen_at asynchronously — don't block request
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		sm.pool.Exec(bgCtx, //nolint:errcheck
			`UPDATE sessions SET last_seen_at = now() WHERE id = $1`, s.ID)
	}()

	return &s, nil
}

// Revoke invalidates a specific session immediately.
func (sm *SessionManager) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	_, err := sm.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`,
		sessionID,
	)
	return err
}

// RevokeAll invalidates all sessions for a user (e.g., on password change or compromise).
func (sm *SessionManager) RevokeAll(ctx context.Context, userID uuid.UUID) error {
	_, err := sm.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return err
}

// RevokeAllExcept revokes all sessions for a user except the current one.
// Used when user changes password — keep the current session alive.
func (sm *SessionManager) RevokeAllExcept(ctx context.Context, userID, keepSessionID uuid.UUID) error {
	_, err := sm.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND id != $2 AND revoked_at IS NULL
	`, userID, keepSessionID)
	return err
}

// SetCookie sets the session cookie on the HTTP response.
// Cookie is HttpOnly, Secure, SameSite=Strict as required by security policy.
func (sm *SessionManager) SetCookie(w http.ResponseWriter, rawToken string, expiresAt time.Time, cfg *config.SessionConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    rawToken,
		Path:     "/",
		Domain:   cfg.CookieDomain,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		Secure:   cfg.SecureCookie,
		HttpOnly: true,                // JavaScript cannot access this cookie
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearCookie removes the session cookie from the client.
func (sm *SessionManager) ClearCookie(w http.ResponseWriter, cfg *config.SessionConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   cfg.CookieDomain,
		MaxAge:   -1,
		Secure:   cfg.SecureCookie,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// GetTokenFromRequest extracts the raw session token from the request cookie.
// Returns empty string if no valid cookie is present.
func GetTokenFromRequest(r *http.Request, cookieName string) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	// Basic sanity check — raw token is a 64-char hex string
	if len(cookie.Value) != 64 {
		return ""
	}
	return cookie.Value
}

// CleanExpiredSessions removes expired and revoked sessions older than the given duration.
// Should be called periodically (e.g., daily cron job).
func (sm *SessionManager) CleanExpiredSessions(ctx context.Context, olderThan time.Duration) (int64, error) {
	result, err := sm.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE expires_at < now() - $1::interval
		   OR (revoked_at IS NOT NULL AND revoked_at < now() - $1::interval)
	`, olderThan.String())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// hashToken returns the SHA-256 hex digest of the raw token.
// This is what gets stored in the database.
func hashToken(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(h[:])
}

// extractIP extracts the client IP from the request, handling reverse proxy headers.
func extractIP(r *http.Request) string {
	// X-Forwarded-For is set by reverse proxies — only trust if behind known proxy
	// For simplicity in Phase 2, use RemoteAddr; Phase 16 adds proxy header trust
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Session errors
var (
	ErrSessionInvalid = fmt.Errorf("session is invalid or expired")
	ErrSessionRevoked = fmt.Errorf("session has been revoked")
)
