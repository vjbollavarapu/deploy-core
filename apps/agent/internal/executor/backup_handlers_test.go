package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestDispatchRegistry_ContainsBackupOps(t *testing.T) {
	r := buildRegistry(nil, nil, nil, "", nil, nil)

	ops := []string{
		protocol.OpCreateBackup,
		protocol.OpRestoreBackup,
	}
	for _, op := range ops {
		if _, ok := r[op]; !ok {
			t.Errorf("expected registry to contain %s", op)
		}
	}
}

func TestBackupHandlers_NilClientFailsGracefully(t *testing.T) {
	tmpDir := t.TempDir()

	createH := createBackupHandler(nil, nil, tmpDir, nil)
	_, err := createH.Execute(context.Background(), map[string]any{"backupId": "123", "databaseId": "db-1"})
	if err == nil {
		t.Errorf("expected error with nil docker client, got nil")
	}
	if execErr, ok := err.(*ExecutionError); !ok || execErr.Code != ErrCodeDockerError {
		t.Errorf("expected ErrCodeDockerError, got %v", err)
	}

	restoreH := restoreBackupHandler(nil, nil, tmpDir, nil)
	_, err = restoreH.Execute(context.Background(), map[string]any{
		"restoreId": "123", "backupId": "123", "targetDatabaseId": "db-1",
	})
	if err == nil {
		t.Errorf("expected error with nil docker client, got nil")
	}
}
