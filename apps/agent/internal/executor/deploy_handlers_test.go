package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestDeployRevisionHandler_ValidationAndDispatch(t *testing.T) {
	cli := &docker.Client{}
	reg := buildRegistry(cli, nil, nil, "", nil, nil)

	handler, ok := reg[protocol.OpDeployRevision]
	if !ok {
		t.Fatalf("expected OpDeployRevision to be registered")
	}

	// Test nil payload
	_, err := handler.Execute(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error on nil payload, got nil")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeInvalidPayload {
		t.Fatalf("expected ErrCodeInvalidPayload, got %v", err)
	}

	// Test missing required fields
	_, err = handler.Execute(context.Background(), map[string]any{
		"applicationId": "app-1",
	})
	if err == nil {
		t.Fatalf("expected error on missing fields, got nil")
	}
	execErr, ok = err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeInvalidPayload {
		t.Fatalf("expected ErrCodeInvalidPayload, got %v", err)
	}

	// Nil docker client must fail — never report simulated deployment success.
	nilReg := buildRegistry(nil, nil, nil, "", nil, nil)
	nilHandler := nilReg[protocol.OpDeployRevision]
	_, err = nilHandler.Execute(context.Background(), map[string]any{
		"applicationId": "app-123",
		"image":         "redis:7-alpine",
	})
	if err == nil {
		t.Fatalf("expected error for nil docker client")
	}
	execErr, ok = err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}
