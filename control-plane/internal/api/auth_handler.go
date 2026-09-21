// Package api — auth HTTP handlers.
// Handles login, logout, session revocation.
// All sensitive inputs are treated carefully — NEVER logged.
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/config"
	"github.com/els3aty/goha-webpanel/control-plane/internal/middleware"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	pool         *pgxpool.Pool
	sessions     *auth.SessionManager
	totp         *auth.TOTPManager
	auditLog     *audit.Logger
	cfg          *config.Config
	loginLimiter *middleware.RateLimiter
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	pool *pgxpool.Pool,
	sessions *auth.SessionManager,
	totp *auth.TOTPManager,
	auditLog *audit.Logger,
	cfg *config.Config,
) *AuthHandler {
	return &AuthHandler{
		pool:         pool,
		sessions:     sessions,
		totp:         totp,
		auditLog:     auditLog,
		cfg:          cfg,
		loginLimiter: middleware.LoginRateLimiter(),
	}
}

// ── Request/Response types ────────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"` // optional — provided when MFA enabled
}

type loginResponse struct {
	Success     bool   `json:"success"`
	MFARequired bool   `json:"mfa_required,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	Role        string `json:"role,omitempty"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// Login handles POST /api/auth/login
// Rate-limited per IP. Returns generic error to prevent user enumeration.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Rate limit check — per IP
	ip := extractClientIP(r)
	if !h.loginLimiter.Allow(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "too many login attempts, please wait before trying again",
		})
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Basic input validation
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		// Generic error — don't distinguish between bad email and bad password
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "invalid credentials",
		})
		h.logLoginFailure(r, "", "missing_credentials")
		return
	}

	// Fetch user — NOTE: same error returned whether user exists or not
	var (
		userID      uuid.UUID
		role        string
		storedHash  string
		mfaEnabled  bool
		isActive    bool
		isSuspended bool
	)

	err := h.pool.QueryRow(r.Context(), `
		SELECT id, role, password_hash, mfa_enabled, is_active, is_suspended
		FROM users
		WHERE email = $1
	`, req.Email).Scan(&userID, &role, &storedHash, &mfaEnabled, &isActive, &isSuspended)

	if err != nil {
		// User not found — still verify a dummy hash to prevent timing attacks
		auth.VerifyPassword(req.Password, dummyHash) //nolint:errcheck
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		h.logLoginFailure(r, "", "user_not_found")
		return
	}

	// Verify password (Argon2id — constant time)
	match, err := auth.VerifyPassword(req.Password, storedHash)
	if err != nil || !match {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		h.logLoginFailure(r, userID.String(), "bad_password")
		return
	}

	// Check account status
	if !isActive {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "account is not active"})
		h.logLoginFailure(r, userID.String(), "account_inactive")
		return
	}
	if isSuspended {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "account is suspended"})
		h.logLoginFailure(r, userID.String(), "account_suspended")
		return
	}

	// MFA check
	mfaVerified := false
	if mfaEnabled {
		if req.TOTPCode == "" {
			// MFA required but not provided — tell client
			writeJSON(w, http.StatusOK, loginResponse{
				Success:     false,
				MFARequired: true,
			})
			return
		}

		// Verify TOTP code
		valid, err := h.totp.Verify(r.Context(), userID, req.TOTPCode)
		if err != nil || !valid {
			// Try backup code
			valid, _ = h.totp.VerifyBackupCode(r.Context(), userID, req.TOTPCode)
		}
		if !valid {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid MFA code"})
			h.logLoginFailure(r, userID.String(), "bad_mfa")
			return
		}
		mfaVerified = true
	}

	// Create session
	rawToken, session, err := h.sessions.Create(r.Context(), userID, r, mfaVerified)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Update last_login_at
	h.pool.Exec(r.Context(), //nolint:errcheck
		`UPDATE users SET last_login_at = now() WHERE id = $1`, userID)

	// Set session cookie
	h.sessions.SetCookie(w, rawToken, session.ExpiresAt, &h.cfg.Session)

	// Audit success
	h.auditLog.LogFromContext(r.Context(), "auth.login", "session", &session.ID,
		audit.ResultSuccess, map[string]any{
			"ip":           ip,
			"mfa_verified": mfaVerified,
		})

	// Return success — do NOT return raw token in body (it's in the cookie)
	writeJSON(w, http.StatusOK, loginResponse{
		Success: true,
		UserID:  userID.String(),
		Role:    role,
	})
}

// Logout handles POST /api/auth/logout
// Revokes the current session and clears the cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
		return
	}

	// Revoke current session
	h.sessions.Revoke(r.Context(), p.SessionID) //nolint:errcheck

	// Clear cookie
	h.sessions.ClearCookie(w, &h.cfg.Session)

	// Audit
	h.auditLog.LogFromContext(r.Context(), "auth.logout", "session", &p.SessionID,
		audit.ResultSuccess, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

// LogoutAll handles POST /api/auth/logout-all
// Revokes ALL sessions for the current user.
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	h.sessions.RevokeAll(r.Context(), p.UserID) //nolint:errcheck
	h.sessions.ClearCookie(w, &h.cfg.Session)

	h.auditLog.LogFromContext(r.Context(), "auth.logout_all", "user", &p.UserID,
		audit.ResultSuccess, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "all sessions revoked"})
}

// Me handles GET /api/auth/me
// Returns the current authenticated user's basic info.
// NEVER returns password hash, TOTP secret, or session token.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	var email, username string
	var mfaEnabled bool
	var createdAt time.Time

	err := h.pool.QueryRow(r.Context(), `
		SELECT email, username, mfa_enabled, created_at
		FROM users WHERE id = $1
	`, p.UserID).Scan(&email, &username, &mfaEnabled, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          p.UserID.String(),
		"email":       email,
		"username":    username,
		"role":        string(p.Role),
		"mfa_enabled": mfaEnabled,
		"mfa_verified": p.MFAVerified,
		"created_at":  createdAt,
		// deliberately NOT returning: password_hash, totp_secret, session_id
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// dummyHash is used for constant-time comparison when user is not found.
// This prevents timing attacks that could enumerate valid email addresses.
// IMPORTANT: This must be computed at startup, not on every request.
var dummyHash = func() string {
	h, _ := auth.HashPassword("dummy_password_for_timing", auth.DefaultParams)
	return h
}()

func (h *AuthHandler) logLoginFailure(r *http.Request, userID, reason string) {
	meta := map[string]any{"reason": reason, "ip": extractClientIP(r)}
	reqID := middleware.RequestIDFromContext(r.Context())
	_ = reqID // used in the event
	h.auditLog.Log(r.Context(), audit.Event{
		Action:    "auth.login",
		Result:    audit.ResultFailure,
		IPAddress: extractClientIP(r),
		RequestID: middleware.RequestIDFromContext(r.Context()),
		Metadata:  meta,
		// NOTE: We log the failure reason but NOT the password or email
	})
}

func extractClientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[:idx]
	}
	return addr
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
