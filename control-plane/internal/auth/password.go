// Package auth provides password hashing using Argon2id.
// Argon2id is the current best practice for password hashing (OWASP recommendation).
//
// SECURITY: Passwords are NEVER logged, returned in API responses, or stored in plaintext.
// Only the Argon2id hash is stored in the database.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params holds the parameters for Argon2id hashing.
// These meet OWASP minimum recommendations.
type Argon2Params struct {
	Memory      uint32 // in KiB — minimum 64MB (65536)
	Iterations  uint32 // minimum 3
	Parallelism uint8  // threads
	SaltLength  uint32 // bytes — 16 minimum
	KeyLength   uint32 // output hash length in bytes — 32 minimum
}

// DefaultParams returns the recommended Argon2id parameters.
// Adjust Memory/Iterations based on server capability while keeping
// hash time around 500ms on the target hardware.
var DefaultParams = &Argon2Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// ErrInvalidHash is returned when a hash string is malformed.
var ErrInvalidHash = errors.New("invalid password hash format")

// ErrIncompatibleVersion is returned when the hash uses an unsupported Argon2 version.
var ErrIncompatibleVersion = errors.New("incompatible Argon2 version")

// HashPassword creates an Argon2id hash of the given password.
// Returns a PHC-format string safe for database storage.
// NEVER log the password parameter.
func HashPassword(password string, params *Argon2Params) (string, error) {
	if params == nil {
		params = DefaultParams
	}

	// Generate cryptographically random salt
	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Compute Argon2id hash
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	// Encode as PHC format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		b64Salt,
		b64Hash,
	)

	return encoded, nil
}

// VerifyPassword checks whether the given password matches the stored hash.
// Uses constant-time comparison to prevent timing attacks.
// Returns true if the password is correct, false otherwise.
// NEVER log the password parameter.
func VerifyPassword(password, encodedHash string) (bool, error) {
	params, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}

	// Recompute hash with same parameters
	candidateHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	// Constant-time comparison — prevents timing attacks
	match := subtle.ConstantTimeCompare(hash, candidateHash) == 1
	return match, nil
}

// NeedsRehash returns true if the stored hash was created with weaker parameters
// than the current defaults and should be upgraded on next successful login.
func NeedsRehash(encodedHash string, current *Argon2Params) (bool, error) {
	if current == nil {
		current = DefaultParams
	}
	params, _, _, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}
	return params.Memory < current.Memory ||
		params.Iterations < current.Iterations ||
		params.Parallelism < current.Parallelism, nil
}

// decodeHash parses a PHC-format Argon2id hash string.
func decodeHash(encoded string) (*Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return nil, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleVersion
	}

	params := &Argon2Params{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d",
		&params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return nil, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	params.SaltLength = uint32(len(salt))

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	params.KeyLength = uint32(len(hash))

	return params, salt, hash, nil
}
