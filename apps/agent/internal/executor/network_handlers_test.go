package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestNetworkHandlers_CreateInspectRemove(t *testing.T) {
	// Create client with dummy/offline cli
	cli := &docker.Client{}
	reg := buildRegistry(cli, nil, nil, "", nil, nil)

	createHandler, ok := reg[protocol.OpCreateNetwork]
	if !ok {
		t.Fatalf("OpCreateNetwork not registered in dispatch registry")
	}

	removeHandler, ok := reg[protocol.OpRemoveNetwork]
	if !ok {
		t.Fatalf("OpRemoveNetwork not registered in dispatch registry")
	}

	inspectHandler, ok := reg[protocol.OpInspectNetwork]
	if !ok {
		t.Fatalf("OpInspectNetwork not registered in dispatch registry")
	}

	// Test invalid payload validations
	_, err := createHandler.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Errorf("expected error for empty create network payload, got nil")
	}

	_, err = removeHandler.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Errorf("expected error for empty remove network payload, got nil")
	}

	_, err = inspectHandler.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Errorf("expected error for empty inspect network payload, got nil")
	}
}
