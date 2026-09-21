package agentclient_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hosting-panel/control-plane/internal/agentclient"
)

func TestTaskBuilder_Sign(t *testing.T) {
	nodeID := "node-1"
	signingKey := []byte("super-secret-test-key-32-bytes-long")
	builder := agentclient.NewTaskBuilder(nodeID, signingKey, 5*time.Minute)

	payload := map[string]string{"foo": "bar"}
	task, err := builder.Build("req-123", "GetSystemMetrics", payload, "idemp-key-1")
	if err != nil {
		t.Fatalf("failed to build task: %v", err)
	}

	if task.Envelope.NodeID != nodeID {
		t.Errorf("expected nodeID %s, got %s", nodeID, task.Envelope.NodeID)
	}
	if task.Operation != "GetSystemMetrics" {
		t.Errorf("expected Operation GetSystemMetrics, got %s", task.Operation)
	}
	if task.Signature == "" {
		t.Errorf("expected signature to be populated")
	}

	// Payload must be properly serialized
	var decoded map[string]string
	if err := json.Unmarshal(task.Payload, &decoded); err != nil {
		t.Fatalf("payload was not valid json: %v", err)
	}
	if decoded["foo"] != "bar" {
		t.Errorf("expected foo=bar in payload")
	}

	// Check canonical format implicitly by generating two identical tasks (except for nonce/timestamps)
	// and ensuring signatures don't panic or fail
	task2, _ := builder.Build("req-124", "GetSystemMetrics", payload, "idemp-key-2")
	if task.Signature == task2.Signature {
		t.Error("signatures for different tasks should be unique (thanks to nonce)")
	}
}
