package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestActivateRevisionHandler_Validation(t *testing.T) {
	h := activateRevisionHandler(nil, nil)

	// Missing candidate ID and name
	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatalf("expected error for empty payload")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeInvalidPayload {
		t.Fatalf("expected ErrCodeInvalidPayload, got %v", err)
	}
}

func TestActivateRevisionHandler_NilDockerFails(t *testing.T) {
	h := activateRevisionHandler(nil, nil)

	_, err := h.Execute(context.Background(), map[string]any{
		"candidateContainerId": "cand-sim-1",
		"proxyNetwork":         "deploycore-proxy",
		"oldContainerId":       "old-sim-1",
	})
	if err == nil {
		t.Fatalf("expected error for nil docker client")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}

func TestDeployRevisionHandler_DelegatesToActivationOnPhase(t *testing.T) {
	h := deployRevisionHandler(nil, nil)

	// DeployRevision with phase: enable_routing must not stub-succeed without Docker.
	_, err := h.Execute(context.Background(), map[string]any{
		"phase":         "enable_routing",
		"containerName": "dc-app-r2-1",
	})
	if err == nil {
		t.Fatalf("expected error for nil docker client on enable_routing")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}

	_ = protocol.OpDeployRevision // keep import stable if unused after edit
}
