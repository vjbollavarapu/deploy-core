package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestHealthHandlers_ValidationAndDispatch(t *testing.T) {
	cli := &docker.Client{}
	reg := buildRegistry(cli, nil, nil, "", nil, nil)

	handler, ok := reg[protocol.OpRunHealthCheck]
	if !ok {
		t.Fatalf("OpRunHealthCheck not registered in dispatch registry")
	}

	// 1. Missing containerId / containerName
	_, err := handler.Execute(context.Background(), map[string]any{
		"probeType": "HTTP",
		"port":      8080,
	})
	if err == nil {
		t.Errorf("expected error for missing container identifier, got nil")
	}

	// 2. Nil docker client must fail — never report synthetic healthy.
	nilReg := buildRegistry(nil, nil, nil, "", nil, nil)
	nilHandler := nilReg[protocol.OpRunHealthCheck]
	_, err = nilHandler.Execute(context.Background(), map[string]any{
		"containerName": "dc-daya-r1-1",
		"probeType":     "HTTP",
		"port":          8080,
		"path":          "/healthz",
	})
	if err == nil {
		t.Fatalf("expected error for nil docker client")
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ErrCodeDockerError {
		t.Fatalf("expected ErrCodeDockerError, got %v", err)
	}
}
