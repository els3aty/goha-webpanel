package agentclient

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TaskBuilder helps construct and sign tasks securely.
type TaskBuilder struct {
	nodeID     string
	signingKey []byte
	ttl        time.Duration
}

// NewTaskBuilder creates a new task builder for a specific node.
func NewTaskBuilder(nodeID string, signingKey []byte, ttl time.Duration) *TaskBuilder {
	return &TaskBuilder{
		nodeID:     nodeID,
		signingKey: signingKey,
		ttl:        ttl,
	}
}

// Build creates a fully populated, signed Task ready for transmission.
func (b *TaskBuilder) Build(requestID string, operation string, payload interface{}, idempotencyKey string) (*Task, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	nonce := generateNonce()

	task := &Task{
		Envelope: Envelope{
			Version:        Version,
			TaskID:         uuid.New().String(),
			RequestID:      requestID,
			NodeID:         b.nodeID,
			IssuedAt:       now.Format(time.RFC3339),
			ExpiresAt:      now.Add(b.ttl).Format(time.RFC3339),
			Nonce:          nonce,
			IdempotencyKey: idempotencyKey,
		},
		Operation: operation,
		Payload:   payloadBytes,
	}

	task.Signature = b.sign(task)
	return task, nil
}

// sign computes the HMAC-SHA256 signature exactly as the agent expects it.
// The canonical form covers all security-relevant fields in a specific order.
func (b *TaskBuilder) sign(task *Task) string {
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

	mac := hmac.New(sha256.New, b.signingKey)
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// generateNonce creates a 32-byte cryptographically random hex string.
func generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
