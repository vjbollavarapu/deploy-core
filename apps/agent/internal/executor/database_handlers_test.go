package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestDispatchRegistry_ContainsDatabaseOps(t *testing.T) {
	r := buildRegistry(nil, nil, nil, "", nil, nil)

	ops := []string{
		protocol.OpProvisionDatabase,
		protocol.OpStartDatabase,
		protocol.OpStopDatabase,
	}
	for _, op := range ops {
		if _, ok := r[op]; !ok {
			t.Errorf("expected registry to contain %s", op)
		}
	}
}

func TestDatabaseHandlers_NilClientFailsGracefully(t *testing.T) {
	provH := provisionDatabaseHandler(nil, nil, nil)
	_, err := provH.Execute(context.Background(), map[string]any{"databaseId": "db-1"})
	if err == nil {
		t.Errorf("expected error with nil docker client, got nil")
	}
	if execErr, ok := err.(*ExecutionError); !ok || execErr.Code != ErrCodeDockerError {
		t.Errorf("expected ErrCodeDockerError, got %v", err)
	}

	startH := startDatabaseHandler(nil, nil)
	_, err = startH.Execute(context.Background(), map[string]any{"databaseId": "123"})
	if err == nil {
		t.Errorf("expected error with nil docker client, got nil")
	}

	stopH := stopDatabaseHandler(nil, nil)
	_, err = stopH.Execute(context.Background(), map[string]any{"databaseId": "123"})
	if err == nil {
		t.Errorf("expected error with nil docker client, got nil")
	}
}
