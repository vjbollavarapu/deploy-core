package executor

import (
	"context"
	"testing"
)

func TestStopContainerHandler_Validation(t *testing.T) {
	h := stopContainerHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatalf("expected error for empty container stop payload")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeInvalidPayload {
		t.Fatalf("expected ErrCodeInvalidPayload, got %v", err)
	}
}

func TestStopContainerHandler_NilDockerFailsClosed(t *testing.T) {
	h := stopContainerHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"containerId":        "dc-sim-1",
		"drainRouting":       true,
		"terminationTimeout": "20s",
	})
	if err == nil {
		t.Fatal("expected DOCKER_ERROR when docker client is nil")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}

func TestStopContainerHandler_NilDockerForceKillFailsClosed(t *testing.T) {
	h := stopContainerHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"containerName": "dc-stuck-1",
		"forceKill":     true,
	})
	if err == nil {
		t.Fatal("expected DOCKER_ERROR when docker client is nil")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}
