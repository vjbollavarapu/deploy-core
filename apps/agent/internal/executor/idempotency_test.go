package executor

import (
	"context"
	"testing"
)

func TestIdempotency_ContainerOperations_NilDockerFails(t *testing.T) {
	hStart := startContainerHandler(nil)
	_, err := hStart.Execute(context.Background(), map[string]any{
		"containerId": "dc-app-test",
	})
	if err == nil {
		t.Fatalf("expected start error for nil docker client")
	}
	if execErr, ok := err.(*ExecutionError); !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}

	hStop := stopContainerHandler(nil, nil)
	_, err = hStop.Execute(context.Background(), map[string]any{
		"containerId": "dc-app-test",
	})
	if err == nil {
		t.Fatalf("expected stop error for nil docker client")
	}
	if execErr, ok := err.(*ExecutionError); !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}
