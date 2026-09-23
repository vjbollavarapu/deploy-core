package restore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/backupstorage"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

type mockRestoreDockerClient struct {
	container     docker.ContainerDetail
	executedCmds  [][]string
	executedEnvs  [][]string
	receivedStdin []byte
	simulatedExit int
	simulatedErr  string
}

func (m *mockRestoreDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	return m.container, nil
}

func (m *mockRestoreDockerClient) ExecContainerWithIO(ctx context.Context, id string, cmd []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	m.executedCmds = append(m.executedCmds, cmd)
	m.executedEnvs = append(m.executedEnvs, env)

	if stdin != nil {
		m.receivedStdin, _ = io.ReadAll(stdin)
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
		t.Fatalf("failed to create storage: %v", err)
	}

	dumpContent := []byte("PGDMP-dump-bytes-for-restore")
	hasher := sha256.New()
	hasher.Write(dumpContent)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	backupID := "bak_test_123"
	_, err = storage.Save(context.Background(), backupID, bytes.NewReader(dumpContent))
	if err != nil {
		t.Fatalf("failed to save mock backup: %v", err)
	}

	cli := &mockRestoreDockerClient{
		container: docker.ContainerDetail{
			ID:    "cnt-db-1",
			Name:  "/dc-db-123",
			State: docker.ContainerState{Running: true, Status: "running"},
			Labels: map[string]string{
				"deploycore.managed":      "true",
				"deploycore.service_type": "database",
				"deploycore.database_id":  "123",
			},
		},
	}

	exec := NewExecutor(cli, storage, nil)

	req := Request{
		RestoreID:         "rest_001",
		BackupID:          backupID,
		TargetDatabaseID:  "123",
		DatabaseName:      "app_prod",
		Username:          "postgres",
		Password:          "secret_pwd",
		ExpectedChecksum:  checksum,
		RequireValidation: true,
	}

	res, err := exec.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !res.Restored {
		t.Errorf("expected restored true")
	}
	if !res.ValidationPassed {
		t.Errorf("expected validationPassed true")
	}

	// Verify stdin received matches dump content
	if !bytes.Equal(cli.receivedStdin, dumpContent) {
		t.Errorf("expected pg_restore to receive dumpContent on stdin")
	}

	// Verify executed commands (first pg_restore, second pg_isready)
	if len(cli.executedCmds) != 2 {
		t.Fatalf("expected 2 executed commands (pg_restore and pg_isready), got %d", len(cli.executedCmds))
	}
	if cli.executedCmds[0][0] != "pg_restore" {
		t.Errorf("expected pg_restore, got %v", cli.executedCmds[0])
	}
	if cli.executedCmds[1][0] != "pg_isready" {
		t.Errorf("expected pg_isready, got %v", cli.executedCmds[1])
	}

	// Verify password isolation: password NOT in command arguments
	for _, cmd := range cli.executedCmds {
		for _, arg := range cmd {
			if strings.Contains(arg, "secret_pwd") {
				t.Errorf("password leaked in command arguments: %s", arg)
			}
		}
	}
}

func TestExecutor_Execute_ChecksumMismatchAborts(t *testing.T) {
	dir := t.TempDir()
	storage, err := backupstorage.NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	dumpContent := []byte("PGDMP-dump-bytes-for-restore")
	backupID := "bak_test_mismatch"
	_, err = storage.Save(context.Background(), backupID, bytes.NewReader(dumpContent))
	if err != nil {
		t.Fatalf("failed to save mock backup: %v", err)
	}

	cli := &mockRestoreDockerClient{
		container: docker.ContainerDetail{
			ID:    "cnt-db-1",
			Name:  "/dc-db-123",
			State: docker.ContainerState{Running: true, Status: "running"},
			Labels: map[string]string{
				"deploycore.managed":      "true",
				"deploycore.service_type": "database",
				"deploycore.database_id":  "123",
			},
		},
	}

	exec := NewExecutor(cli, storage, nil)

	req := Request{
		RestoreID:        "rest_002",
		BackupID:         backupID,
		TargetDatabaseID: "123",
		DatabaseName:     "app_prod",
		Username:         "postgres",
		Password:         "secret_pwd",
		ExpectedChecksum: "0000000000000000000000000000000000000000000000000000000000000000", // deliberate mismatch!
	}

	_, err = exec.Execute(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error on checksum mismatch, got nil")
	}

	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error message, got: %v", err)
	}

	// Verify pg_restore was NEVER executed!
	if len(cli.executedCmds) != 0 {
		t.Errorf("expected zero commands executed upon checksum mismatch, got %d", len(cli.executedCmds))
	}
}
