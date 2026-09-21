// Package protocol defines the agent task protocol:
// task envelope format, signature verification, and expiry/replay protection.
//
// SECURITY INVARIANTS:
//   1. The agent NEVER executes a task that fails signature verification
//   2. The agent NEVER executes an expired task (expires_at < now)
//   3. The agent NEVER executes a replayed task (nonce already seen)
//   4. The agent NEVER executes a task addressed to another node
//   5. These checks run in order — ALL must pass before execution
package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Version is the current protocol version.
// Tasks with a different version are rejected.
const Version = "1"

// Task is the complete signed task envelope sent from Control Plane to Agent.
// The JSON field names are fixed — changing them breaks signature verification.
type Task struct {
	Envelope  Envelope        `json:"envelope"`
	Operation string          `json:"operation"`
	Payload   json.RawMessage `json:"payload"` // operation-specific parameters
	Signature string          `json:"signature"` // HMAC-SHA256 hex of canonical form
}

// Envelope contains task metadata used for authentication and replay protection.
type Envelope struct {
	Version        string `json:"version"`
	TaskID         string `json:"task_id"`
	RequestID      string `json:"request_id"`
	NodeID         string `json:"node_id"` // target node — agent rejects if wrong
	IssuedAt       string `json:"issued_at"`       // RFC3339 UTC
	ExpiresAt      string `json:"expires_at"`      // RFC3339 UTC
	Nonce          string `json:"nonce"`           // 32-byte hex — replay protection
	IdempotencyKey string `json:"idempotency_key"` // safe retry key
}

// TaskResult is the response sent back to the Control Plane after execution.
type TaskResult struct {
	TaskID     string      `json:"task_id"`
	Status     string      `json:"status"` // "succeeded" | "failed" | "rolled_back"
	Output     interface{} `json:"output,omitempty"`
	Error      *TaskError  `json:"error,omitempty"`
	ExecutedAt string      `json:"executed_at"`
	DurationMS int64       `json:"duration_ms"`
}

// TaskError is returned in TaskResult when execution failed.
type TaskError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Verification errors — specific types for logging and monitoring.
var (
	ErrInvalidVersion   = errors.New("unsupported protocol version")
	ErrWrongNode        = errors.New("task addressed to different node")
	ErrExpiredTask      = errors.New("task has expired")
	ErrFutureTask       = errors.New("task issued in the future (clock skew)")
	ErrReplayedTask     = errors.New("task nonce already seen (replay attack)")
	ErrInvalidSignature = errors.New("task signature verification failed")
	ErrEmptyNonce       = errors.New("task nonce is empty")
	ErrEmptyOperation   = errors.New("task operation is empty")
)

// clockSkewTolerance is how far in the future a task's issued_at can be.
const clockSkewTolerance = 30 * time.Second

// Verifier verifies incoming tasks.
type Verifier struct {
	nodeID     string
	signingKey []byte // HMAC-SHA256 key shared with Control Plane
	maxAge     time.Duration
	nonceStore NonceStore
}

// NonceStore tracks seen nonces for replay protection.
type NonceStore interface {
	// HasSeen returns true if the nonce was already processed.
	HasSeen(nonce string) bool
	// MarkSeen records a nonce. TTL controls auto-expiry.
	MarkSeen(nonce string, ttl time.Duration)
}

// NewVerifier creates a task verifier.
// nodeID: this agent's node ID — tasks for other nodes are rejected
// signingKey: HMAC key distributed from Control Plane at enrollment
// maxAge: maximum allowed task age (e.g., 5 minutes)
// nonceStore: implementation of replay protection store
func NewVerifier(nodeID string, signingKey []byte, maxAge time.Duration, store NonceStore) *Verifier {
	return &Verifier{
		nodeID:     nodeID,
		signingKey: signingKey,
		maxAge:     maxAge,
		nonceStore: store,
	}
}

// Verify performs all security checks on an incoming task.
// Returns nil only if ALL checks pass. ANY failure returns an error.
//
// Checks (in order):
//  1. Protocol version matches
//  2. Task is addressed to this node
//  3. Task is not expired
//  4. Task is not from the future (clock skew)
//  5. Nonce is non-empty
//  6. Operation is non-empty
//  7. Signature is valid (HMAC-SHA256)
//  8. Nonce has not been seen before (replay)
//  9. Mark nonce as seen
func (v *Verifier) Verify(task *Task) error {
	e := &task.Envelope

	// 1. Protocol version
	if e.Version != Version {
		return fmt.Errorf("%w: got %q, expected %q", ErrInvalidVersion, e.Version, Version)
	}

	// 2. Node ID — task must be for THIS node
	if e.NodeID != v.nodeID {
		// Log attempt but don't reveal our node ID in the error
		return ErrWrongNode
	}

	// 3. Parse timestamps
	issuedAt, err := time.Parse(time.RFC3339, e.IssuedAt)
	if err != nil {
		return fmt.Errorf("invalid issued_at timestamp: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, e.ExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid expires_at timestamp: %w", err)
	}

	now := time.Now().UTC()

	// 4. Check expiry
	if now.After(expiresAt) {
		return fmt.Errorf("%w: expired at %s, now %s", ErrExpiredTask, expiresAt, now)
	}

	// 5. Check issued_at is not too far in the future
	if issuedAt.After(now.Add(clockSkewTolerance)) {
		return fmt.Errorf("%w: issued at %s, now %s", ErrFutureTask, issuedAt, now)
	}

	// 6. Nonce must be present
	if e.Nonce == "" {
		return ErrEmptyNonce
	}

	// 7. Operation must be present
	if task.Operation == "" {
		return ErrEmptyOperation
	}

	// 8. Verify HMAC-SHA256 signature
	// IMPORTANT: Do this before checking nonce store to avoid oracle attacks
	expectedSig := computeSignature(task, v.signingKey)
	if !hmac.Equal([]byte(task.Signature), []byte(expectedSig)) {
		// Do NOT reveal expected signature in the error
		return ErrInvalidSignature
	}

	// 9. Replay protection — check AFTER signature verification
	if v.nonceStore.HasSeen(e.Nonce) {
		return ErrReplayedTask
	}

	// 10. Record nonce — TTL = 2 × max task age
	v.nonceStore.MarkSeen(e.Nonce, v.maxAge*2)

	return nil
}

// computeSignature computes the HMAC-SHA256 signature for a task.
// The canonical form covers all fields that the agent acts on.
// Adding or removing fields from canonical form is a breaking protocol change.
func computeSignature(task *Task, key []byte) string {
	// Canonical string: deterministic, covers all security-relevant fields
	canonical := strings.Join([]string{
		task.Envelope.Version,
		task.Envelope.TaskID,
		task.Envelope.NodeID,
		task.Envelope.IssuedAt,
		task.Envelope.ExpiresAt,
		task.Envelope.Nonce,
		task.Operation,
		string(task.Payload),
	}, "\n")

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// Sign creates a signature for a task (used by Control Plane, not agent).
// Included here for testing and integration purposes.
func Sign(task *Task, key []byte) string {
	return computeSignature(task, key)
}

// ParseTask decodes a JSON task and validates its structure.
// Does NOT verify the signature — call Verifier.Verify() for that.
func ParseTask(data []byte) (*Task, error) {
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to parse task: %w", err)
	}
	// Basic structural checks
	if task.Envelope.TaskID == "" {
		return nil, fmt.Errorf("task_id is missing")
	}
	if task.Envelope.NodeID == "" {
		return nil, fmt.Errorf("node_id is missing")
	}
	return &task, nil
}

// ParsePayload decodes the typed operation payload into the given struct.
// Operations must call this to get their typed parameters.
func ParsePayload[T any](task *Task) error {
	var params T
	_ = params
	if err := json.Unmarshal(task.Payload, &params); err != nil {
		return fmt.Errorf("failed to parse payload for operation %s: %w",
			task.Operation, err)
	}
	return nil
}

// DecodePayload unmarshals payload directly into the provided destination.
func DecodePayload(payload []byte, dest any) error {
	return json.Unmarshal(payload, dest)
}
