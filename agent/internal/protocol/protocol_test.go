package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/els3aty/goha-webpanel/agent/internal/nonce"
	"github.com/els3aty/goha-webpanel/agent/internal/protocol"
)

var testKey = []byte("super-secret-test-key-32-bytes-long")

func createValidTask() *protocol.Task {
	now := time.Now().UTC()
	return &protocol.Task{
		Envelope: protocol.Envelope{
			Version:   protocol.Version,
			TaskID:    "task-123",
			NodeID:    "node-1",
			IssuedAt:  now.Format(time.RFC3339),
			ExpiresAt: now.Add(5 * time.Minute).Format(time.RFC3339),
			Nonce:     "random-nonce-12345",
		},
		Operation: "GetSystemMetrics",
		Payload:   json.RawMessage(`{}`),
	}
}

func TestVerify_ValidTask(t *testing.T) {
	store := nonce.New()
	verifier := protocol.NewVerifier("node-1", testKey, 5*time.Minute, store)

	task := createValidTask()
	task.Signature = protocol.Sign(task, testKey)

	if err := verifier.Verify(task); err != nil {
		t.Fatalf("expected valid task, got err: %v", err)
	}

	// Verify nonce was stored
	if !store.HasSeen(task.Envelope.Nonce) {
		t.Error("expected nonce to be recorded in store")
	}
}

func TestVerify_InvalidSignature(t *testing.T) {
	store := nonce.New()
	verifier := protocol.NewVerifier("node-1", testKey, 5*time.Minute, store)

	task := createValidTask()
	task.Signature = protocol.Sign(task, []byte("wrong-key")) // Forged

	if err := verifier.Verify(task); err != protocol.ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerify_ReplayAttack(t *testing.T) {
	store := nonce.New()
	verifier := protocol.NewVerifier("node-1", testKey, 5*time.Minute, store)

	task := createValidTask()
	task.Signature = protocol.Sign(task, testKey)

	// First execution should pass
	if err := verifier.Verify(task); err != nil {
		t.Fatalf("first verify failed: %v", err)
	}

	// Second execution MUST fail
	if err := verifier.Verify(task); err != protocol.ErrReplayedTask {
		t.Errorf("expected ErrReplayedTask on replay, got %v", err)
	}
}

func TestVerify_ExpiredTask(t *testing.T) {
	store := nonce.New()
	verifier := protocol.NewVerifier("node-1", testKey, 5*time.Minute, store)

	task := createValidTask()
	// Set expiry in the past
	task.Envelope.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	task.Signature = protocol.Sign(task, testKey)

	if err := verifier.Verify(task); !strings.Contains(err.Error(), "expired at") {
		t.Errorf("expected expired error, got %v", err)
	}
}

func TestVerify_WrongNode(t *testing.T) {
	store := nonce.New()
	// Verifier configured for node-2
	verifier := protocol.NewVerifier("node-2", testKey, 5*time.Minute, store)

	task := createValidTask() // Addressed to node-1
	task.Signature = protocol.Sign(task, testKey)

	if err := verifier.Verify(task); err != protocol.ErrWrongNode {
		t.Errorf("expected ErrWrongNode, got %v", err)
	}
}

func TestVerify_MalleabilityAttack(t *testing.T) {
	store := nonce.New()
	verifier := protocol.NewVerifier("node-1", testKey, 5*time.Minute, store)

	task := createValidTask()
	task.Signature = protocol.Sign(task, testKey)

	// Attacker modifies the operation
	task.Operation = "DeleteDatabase"

	if err := verifier.Verify(task); err != protocol.ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature after modification, got %v", err)
	}
}

func TestVerify_FutureTask(t *testing.T) {
	store := nonce.New()
	verifier := protocol.NewVerifier("node-1", testKey, 5*time.Minute, store)

	task := createValidTask()
	// Issued 1 hour in the future (beyond clock skew)
	task.Envelope.IssuedAt = time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	task.Signature = protocol.Sign(task, testKey)

	if err := verifier.Verify(task); !strings.Contains(err.Error(), "issued in the future") {
		t.Errorf("expected future task error, got %v", err)
	}
}
