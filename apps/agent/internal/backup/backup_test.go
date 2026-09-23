package backup

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/backupstorage"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

type mockExecDockerClient struct {
	container     docker.ContainerDetail
	executedCmd   []string
	executedEnv   []string
	simulatedExit int
	simulatedOut  []byte
	simulatedErr  string
}

func (m *mockExecDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	return m.container, nil
}

func (m *mockExecDockerClient) ExecContainerWithIO(ctx context.Context, id string, cmd []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	m.executedCmd = cmd
	m.executedEnv = env

	if stdout != nil && len(m.simulatedOut) > 0 {
		_, _ = stdout.Write(m.simulatedOut)
	}
	if stderr != nil && m.simulatedErr != "" {
		_, _ = stderr.Write([]byte(m.simulatedErr))
	}
	return m.simulatedExit, nil
}

func TestExecutor_Execute_Success(t *testing.T) {
	dir := t.TempDir()
	storage, err := backupstorage.NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	mockDumpData := []byte("PGDMP-sample-binary-format-content-stream")
	cli := &mockExecDockerClient{
		container: docker.ContainerDetail{
			ID:    "c-1",
			Name:  "/dc-db-123",
			State: docker.ContainerState{Running: true, Status: "running"},
			Labels: map[string]string{
				"deploycore.managed":      "true",
				"deploycore.service_type": "database",
				"deploycore.database_id":  "123",
			},
		},
		simulatedOut: mockDumpData,
	}

	exec := NewExecutor(cli, storage, nil)
	req := Request{
		BackupID:     "bak_test_ok",
		DatabaseID:   "123",
		DatabaseName: "main_db",
		Username:     "postgres",
		Password:     "super_secret_pwd",
	}

	res, err := exec.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if res.BackupID != "bak_test_ok" {
		t.Errorf("expected backupId 'bak_test_ok', got %s", res.BackupID)
	}
	if res.SizeBytes != int64(len(mockDumpData)) {
		t.Errorf("expected size %d, got %d", len(mockDumpData), res.SizeBytes)
	}
	if res.Checksum == "" {
		t.Errorf("expected non-empty checksum")
	}

	// Verify command and env
	if len(cli.executedCmd) == 0 || cli.executedCmd[0] != "pg_dump" {
		t.Errorf("expected pg_dump cmd, got: %v", cli.executedCmd)
	}

	// Invariant: password MUST NOT appear in command line arguments!
	for _, arg := range cli.executedCmd {
		if strings.Contains(arg, "super_secret_pwd") {
			t.Errorf("password leaked in command line args: %s", arg)
		}
	}

	// Password must be in exec env
	foundEnv := false
	for _, e := range cli.executedEnv {
		if e == "PGPASSWORD=super_secret_pwd" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Errorf("expected PGPASSWORD in exec env")
	}

	// Verify stored file matches
	exists, err := storage.Exists(context.Background(), "bak_test_ok")
	if err != nil || !exists {
		t.Errorf("expected backup to exist in storage")
	}
}

func TestExecutor_Execute_FailureCleansStorage(t *testing.T) {
	dir := t.TempDir()
	storage, err := backupstorage.NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	cli := &mockExecDockerClient{
		container: docker.ContainerDetail{
			ID:    "c-1",
			Name:  "/dc-db-123",
			State: docker.ContainerState{Running: true, Status: "running"},
			Labels: map[string]string{
				"deploycore.managed":      "true",
				"deploycore.service_type": "database",
				"deploycore.database_id":  "123",
			},
		},
		simulatedExit: 1,
		simulatedErr:  "pg_dump: error: password authentication failed for user secret_pass",
	}

	exec := NewExecutor(cli, storage, nil)
	req := Request{
		BackupID:     "bak_test_fail",
		DatabaseID:   "123",
		DatabaseName: "main_db",
		Username:     "postgres",
		Password:     "secret_pass",
	}

	_, err = exec.Execute(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error on exitCode 1, got nil")
	}

	// Verify password was redacted from error message
	if strings.Contains(err.Error(), "secret_pass") {
		t.Errorf("password leaked in error message: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("expected [REDACTED] in error message: %v", err)
	}

	// Verify backup artifact was cleaned up
	exists, _ := storage.Exists(context.Background(), "bak_test_fail")
	if exists {
		t.Errorf("expected failed backup artifact to be cleaned from storage")
	}
}
