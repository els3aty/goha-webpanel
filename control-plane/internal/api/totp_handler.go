// Package api — TOTP handler.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/config"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TOTPHandler handles MFA enrollment and verification.
type TOTPHandler struct {
	pool     *pgxpool.Pool
	totp     *auth.TOTPManager
	auditLog *audit.Logger
	cfg      *config.Config
	// enrollmentStore holds pending enrollments (secret + backup codes) temporarily
	// Key: userID string, Value: *TOTPEnrollment
	// In production: use Redis with TTL; for Phase 2: in-memory with caution
	pendingEnrollments map[string]*auth.TOTPEnrollment
}

// NewTOTPHandler creates a new TOTPHandler.
func NewTOTPHandler(
	pool *pgxpool.Pool,
	totp *auth.TOTPManager,
	auditLog *audit.Logger,
	cfg *config.Config,
) *TOTPHandler {
	return &TOTPHandler{
		pool:               pool,
		totp:               totp,
		auditLog:           auditLog,
		cfg:                cfg,
		pendingEnrollments: make(map[string]*auth.TOTPEnrollment),
	}
}

// BeginEnrollment starts the TOTP enrollment flow.
// Returns the QR code URL and backup codes. These are shown ONCE.
// POST /api/auth/totp/enroll
func (h *TOTPHandler) BeginEnrollment(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	enrollment, err := h.totp.GenerateEnrollment(p.Email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Store pending enrollment temporarily (confirm step will use this)
	h.pendingEnrollments[p.UserID.String()] = enrollment

	// Return QR URL and backup codes — SHOWN ONCE, user must save them
	// Note: secret is included for manual entry in authenticator apps
	writeJSON(w, http.StatusOK, map[string]any{
		"qr_url":       enrollment.QRCodeURL,
		"secret":       enrollment.Secret,    // for manual entry
		"backup_codes": enrollment.BackupCodes, // SHOWN ONCE — user must save
		"message":      "Scan the QR code or enter the secret in your authenticator app. Save your backup codes — they will not be shown again.",
	})

	// Note: secret is NOT logged
	h.auditLog.LogFromContext(r.Context(), "auth.totp.enrollment_started", "user", &p.UserID,
		audit.ResultSuccess, nil)
}

// ConfirmEnrollment verifies the first OTP and activates MFA.
// POST /api/auth/totp/confirm
func (h *TOTPHandler) ConfirmEnrollment(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
		return
	}

	enrollment, ok := h.pendingEnrollments[p.UserID.String()]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "no pending enrollment found, please start enrollment again",
		})
		return
	}

	err := h.totp.ConfirmEnrollment(
		r.Context(), p.UserID, enrollment.Secret, req.Code, enrollment.BackupCodes,
	)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		h.auditLog.LogFromContext(r.Context(), "auth.totp.enrollment_failed", "user", &p.UserID,
			audit.ResultFailure, nil)
		return
	}

	// Remove pending enrollment
	delete(h.pendingEnrollments, p.UserID.String())

	h.auditLog.LogFromContext(r.Context(), "auth.totp.enabled", "user", &p.UserID,
		audit.ResultSuccess, nil)

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "MFA enabled successfully",
	})
}

// VerifyMFA verifies a TOTP code for an already-authenticated session.
// Used when session needs MFA elevation.
// POST /api/auth/totp/verify
func (h *TOTPHandler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
		return
	}

	valid, err := h.totp.Verify(r.Context(), p.UserID, req.Code)
	if err != nil || !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid MFA code"})
		h.auditLog.LogFromContext(r.Context(), "auth.mfa.verify_failed", "session", &p.SessionID,
			audit.ResultFailure, nil)
		return
	}

	// Mark session as MFA verified in DB
	h.pool.Exec(r.Context(), //nolint:errcheck
		`UPDATE sessions SET mfa_verified = TRUE WHERE id = $1`, p.SessionID)

	h.auditLog.LogFromContext(r.Context(), "auth.mfa.verified", "session", &p.SessionID,
		audit.ResultSuccess, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "MFA verified"})
}

// DisableMFA removes MFA from the account.
// Requires current TOTP code or backup code to confirm.
// DELETE /api/auth/totp
func (h *TOTPHandler) DisableMFA(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required to disable MFA"})
		return
	}

	// Verify current TOTP code before disabling
	valid, _ := h.totp.Verify(r.Context(), p.UserID, req.Code)
	if !valid {
		valid, _ = h.totp.VerifyBackupCode(r.Context(), p.UserID, req.Code)
	}
	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}

	if err := h.totp.Disable(r.Context(), p.UserID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "auth.totp.disabled", "user", &p.UserID,
		audit.ResultSuccess, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "MFA disabled"})
}
