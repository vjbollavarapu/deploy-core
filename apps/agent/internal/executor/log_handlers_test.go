package executor

import (
	"context"
	"testing"
)

func TestFetchLogsHandler_Validation(t *testing.T) {
	h := fetchLogsHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatalf("expected error for empty fetch logs payload")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeInvalidPayload {
		t.Fatalf("expected ErrCodeInvalidPayload, got %v", err)
	}
}

func TestFetchLogsHandler_NilDockerFailsClosed(t *testing.T) {
	h := fetchLogsHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"containerId": "c-test-1",
		"tail":        "50",
	})
	if err == nil {
		t.Fatal("expected DOCKER_ERROR when docker client is nil")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}

func TestStreamLogsHandler_Validation(t *testing.T) {
	h := streamLogsHandler(nil, nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatalf("expected error for empty stream logs payload")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeInvalidPayload {
		t.Fatalf("expected ErrCodeInvalidPayload, got %v", err)
	}
}

func TestStreamLogsHandler_NilDockerFailsClosed(t *testing.T) {
	h := streamLogsHandler(nil, nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"containerName": "dc-web-r1-1",
		"tail":          "100",
		"timestamps":    true,
	})
	if err == nil {
		t.Fatal("expected DOCKER_ERROR when docker client is nil")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}
