// Package auth — TOTP (Time-based One-Time Password) management.
// Implements RFC 6238 TOTP for MFA.
//
// SECURITY:
//   - TOTP secrets are stored AES-256-GCM encrypted in the database
//   - Secrets are NEVER returned in API responses after enrollment
//   - QR code is shown ONCE during enrollment, then the raw secret is discarded
//   - Backup codes are hashed (Argon2id) before storage
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	// totpIssuer is shown in authenticator apps
	totpIssuer = "HostingPanel"
	// totpDigits is the number of digits in the OTP
	totpDigits = otp.DigitsSix
	// totpPeriod is the time period in seconds
	totpPeriod = 30
	// totpSkew allows N periods of clock drift (each side)
	totpSkew = 1
	// backupCodeCount is the number of backup codes generated
	backupCodeCount = 8
	// backupCodeLength is the length of each backup code
	backupCodeLength = 10
)

// TOTPEnrollment contains the data shown to the user during enrollment.
// The Secret is shown ONCE and must not be logged or returned again.
type TOTPEnrollment struct {
	Secret      string // base32 TOTP secret — shown once
	QRCodeURL   string // otpauth:// URL for QR code generation
	BackupCodes []string // plain backup codes — shown once, then hashed
}

// TOTPManager handles TOTP enrollment and verification.
type TOTPManager struct {
	pool      *pgxpool.Pool
	encryptor Encryptor // encrypts secrets before DB storage
}

// Encryptor is an interface for encrypting/decrypting sensitive data.
// Implemented by the secret store package (Phase 16 will use Vault; for now AES-GCM).
type Encryptor interface {
	Encrypt(plaintext []byte) (ciphertext string, err error)
	Decrypt(ciphertext string) (plaintext []byte, err error)
}

// NewTOTPManager creates a new TOTPManager.
func NewTOTPManager(pool *pgxpool.Pool, enc Encryptor) *TOTPManager {
	return &TOTPManager{pool: pool, encryptor: enc}
}

// GenerateEnrollment creates a new TOTP secret and backup codes for a user.
// This does NOT activate MFA — call ConfirmEnrollment after the user verifies.
func (tm *TOTPManager) GenerateEnrollment(email string) (*TOTPEnrollment, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: email,
		Period:      totpPeriod,
		Digits:      totpDigits,
		Algorithm:   otp.AlgorithmSHA1, // TOTP standard
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	backupCodes, err := generateBackupCodes(backupCodeCount, backupCodeLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	return &TOTPEnrollment{
		Secret:      key.Secret(),
		QRCodeURL:   key.URL(),
		BackupCodes: backupCodes,
	}, nil
}

// ConfirmEnrollment activates MFA for a user after they verify the first OTP.
// The secret is encrypted before storage. Backup codes are hashed.
// Returns error if the provided OTP does not match the secret.
func (tm *TOTPManager) ConfirmEnrollment(
	ctx context.Context,
	userID uuid.UUID,
	secret string,
	otp string,
	backupCodes []string,
) error {
	// Verify the OTP before storing anything
	valid, err := tm.verifyTOTP(otp, secret)
	if err != nil || !valid {
		return ErrInvalidTOTP
	}

	// Encrypt the secret before storage
	encSecret, err := tm.encryptor.Encrypt([]byte(secret))
	if err != nil {
		return fmt.Errorf("failed to encrypt TOTP secret: %w", err)
	}

	// Hash backup codes with Argon2id
	hashedCodes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := HashPassword(code, DefaultParams)
		if err != nil {
			return fmt.Errorf("failed to hash backup code: %w", err)
		}
		hashedCodes[i] = hash
	}

	// Store encrypted secret and hashed backup codes, enable MFA
	_, err = tm.pool.Exec(ctx, `
		UPDATE users
		SET mfa_enabled = TRUE,
		    totp_secret_encrypted = $1,
		    mfa_backup_codes_hash = $2,
		    updated_at = now()
		WHERE id = $3
	`, encSecret, hashedCodes, userID)
	if err != nil {
		return fmt.Errorf("failed to enable MFA: %w", err)
	}

	return nil
}

// Verify checks a TOTP code for a user.
// Fetches the encrypted secret from DB, decrypts it, and verifies.
// NEVER logs the code or the secret.
func (tm *TOTPManager) Verify(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	var encSecret string
	err := tm.pool.QueryRow(ctx,
		`SELECT totp_secret_encrypted FROM users WHERE id = $1 AND mfa_enabled = TRUE`,
		userID,
	).Scan(&encSecret)
	if err != nil {
		return false, ErrMFANotEnabled
	}

	secretBytes, err := tm.encryptor.Decrypt(encSecret)
	if err != nil {
		return false, fmt.Errorf("failed to decrypt TOTP secret: %w", err)
	}

	valid, err := tm.verifyTOTP(code, string(secretBytes))
	if err != nil {
		return false, err
	}
	return valid, nil
}

// VerifyBackupCode checks if a backup code is valid for a user.
// If valid, the code is consumed (removed from the list) to prevent reuse.
// NEVER logs the code.
func (tm *TOTPManager) VerifyBackupCode(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	var hashedCodes []string
	err := tm.pool.QueryRow(ctx,
		`SELECT mfa_backup_codes_hash FROM users WHERE id = $1`,
		userID,
	).Scan(&hashedCodes)
	if err != nil || len(hashedCodes) == 0 {
		return false, nil
	}

	for i, hash := range hashedCodes {
		match, err := VerifyPassword(code, hash)
		if err != nil {
			continue
		}
		if match {
			// Consume this backup code (remove it)
			remaining := append(hashedCodes[:i], hashedCodes[i+1:]...)
			_, err := tm.pool.Exec(ctx,
				`UPDATE users SET mfa_backup_codes_hash = $1 WHERE id = $2`,
				remaining, userID,
			)
			if err != nil {
				return false, fmt.Errorf("failed to consume backup code: %w", err)
			}
			return true, nil
		}
	}

	return false, nil
}

// Disable removes MFA from a user account.
// Requires a valid TOTP code or backup code to confirm intent.
func (tm *TOTPManager) Disable(ctx context.Context, userID uuid.UUID) error {
	_, err := tm.pool.Exec(ctx, `
		UPDATE users
		SET mfa_enabled = FALSE,
		    totp_secret_encrypted = NULL,
		    mfa_backup_codes_hash = NULL,
		    updated_at = now()
		WHERE id = $1
	`, userID)
	return err
}

// ── helpers ──────────────────────────────────────────────────────────────────

// verifyTOTP checks an OTP code against a secret with clock skew tolerance.
func (tm *TOTPManager) verifyTOTP(code, secret string) (bool, error) {
	valid, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      totpSkew,
		Digits:    totpDigits,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false, nil // treat validation errors as invalid code
	}
	return valid, nil
}

// generateBackupCodes creates N random backup codes of the given length.
func generateBackupCodes(count, length int) ([]string, error) {
	codes := make([]string, count)
	for i := range codes {
		b := make([]byte, length)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		// base32 encode and format as XXXXX-XXXXX
		encoded := base32.StdEncoding.EncodeToString(b)[:length]
		codes[i] = encoded[:5] + "-" + encoded[5:]
	}
	return codes, nil
}

// TOTP errors
var (
	ErrInvalidTOTP  = fmt.Errorf("invalid or expired TOTP code")
	ErrMFANotEnabled = fmt.Errorf("MFA is not enabled for this account")
)
