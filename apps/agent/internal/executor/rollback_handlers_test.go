package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestRollbackRevisionHandler_Validation(t *testing.T) {
	h := rollbackRevisionHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatalf("expected error for empty payload")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeInvalidPayload {
		t.Fatalf("expected ErrCodeInvalidPayload, got %v", err)
	}

	_, err = h.Execute(context.Background(), map[string]any{
		"applicationId": "app-1",
		"image":         "app:v1",
	})
	if err == nil {
		t.Fatalf("expected error for missing revisionId")
	}

	_, err = h.Execute(context.Background(), map[string]any{
		"applicationId":    "app-1",
		"targetRevisionId": "rev-1",
	})
	if err == nil {
		t.Fatalf("expected error for missing image")
	}
}

func TestRollbackRevisionHandler_RebuildForbidden(t *testing.T) {
	h := rollbackRevisionHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"applicationId":    "app-1",
		"targetRevisionId": "rev-1",
		"image":            "app:v1",
		"dockerfile":       "Dockerfile",
	})
	if err == nil {
		t.Fatalf("expected error for rebuild attempt during rollback")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeValidation {
		t.Fatalf("expected ErrCodeValidation, got %v", err)
	}
}

func TestRollbackRevisionHandler_NilDockerFails(t *testing.T) {
	h := rollbackRevisionHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"applicationId":    "app-1",
		"targetRevisionId": "rev-1",
		"image":            "app:v1",
	})
	if err == nil {
		t.Fatalf("expected error for nil docker client")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}

func TestDeployRevisionHandler_DelegatesToRollbackOnTrigger(t *testing.T) {
	h := deployRevisionHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"trigger":          "rollback",
		"applicationId":    "app-1",
		"targetRevisionId": "rev-prev",
		"image":            "app:v1",
	})
	if err == nil {
		t.Fatalf("expected error for nil docker client on rollback trigger")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}

func TestDispatchRegistry_ContainsOpRollbackRevision(t *testing.T) {
	reg := buildRegistry(nil, nil, nil, "", nil, nil)

	if _, ok := reg[protocol.OpRollbackRevision]; !ok {
		t.Fatalf("expected OpRollbackRevision to be registered in dispatch registry")
	}
}
