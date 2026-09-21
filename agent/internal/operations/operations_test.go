package operations_test

import (
	"context"
	"testing"

	"github.com/els3aty/goha-webpanel/agent/internal/operations"
)

func TestIsAllowed(t *testing.T) {
	if !operations.IsAllowed(operations.OpCreateHostingUser) {
		t.Error("expected OpCreateHostingUser to be allowed")
	}

	if operations.IsAllowed(operations.OperationName("ExecuteShell")) {
		t.Error("SECURITY: ExecuteShell must NEVER be allowed")
	}
	if operations.IsAllowed(operations.OperationName("RunScript")) {
		t.Error("SECURITY: RunScript must NEVER be allowed")
	}
}

func TestRegistry_RejectsForbiddenRegistration(t *testing.T) {
	registry := operations.NewRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when registering forbidden operation, but code did not panic")
		}
	}()

	registry.Register("ArbitraryCommand", func(ctx context.Context, payload []byte) (interface{}, error) {
		return nil, nil
	})
}

func TestRegistry_Execute(t *testing.T) {
	registry := operations.NewRegistry()

	// Register a valid operation
	called := false
	registry.Register(operations.OpGetSystemMetrics, func(ctx context.Context, payload []byte) (interface{}, error) {
		called = true
		return "metrics", nil
	})

	// Execute valid operation
	res, err := registry.Execute(context.Background(), operations.OpGetSystemMetrics, nil)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if res != "metrics" {
		t.Errorf("expected 'metrics', got %v", res)
	}
	if !called {
		t.Error("handler was not called")
	}

	// Execute missing but allowed operation
	_, err = registry.Execute(context.Background(), operations.OpCreateDatabase, nil)
	if err == nil {
		t.Error("expected error for unimplemented operation, got nil")
	}

	// Execute forbidden operation
	_, err = registry.Execute(context.Background(), "ExecuteShell", nil)
	if err != operations.ErrForbiddenOperation {
		t.Errorf("expected ErrForbiddenOperation, got %v", err)
	}
}
