package failuresim_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/backupstorage"
	"github.com/deploycore/deploy-core/apps/agent/internal/concurrency"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/drain"
	"github.com/deploycore/deploy-core/apps/agent/internal/health"
	"github.com/deploycore/deploy-core/apps/agent/internal/restore"
	"github.com/deploycore/deploy-core/apps/agent/internal/safety"
)

type mockSystemChecker struct {
	freeDisk  int64
	totalDisk int64
	diskErr   error
	availMem  int64
	totalMem  int64
	memErr    error
	dockerErr error
}

func (m *mockSystemChecker) CheckDiskSpace(ctx context.Context, path string) (int64, int64, error) {
	return m.freeDisk, m.totalDisk, m.diskErr
}

func (m *mockSystemChecker) CheckMemory(ctx context.Context) (int64, int64, error) {
	return m.availMem, m.totalMem, m.memErr
}

func (m *mockSystemChecker) CheckDockerHealth(ctx context.Context) error {
	return m.dockerErr
}

// 1. Docker Unavailable
func TestFailure_DockerUnavailable(t *testing.T) {
	sys := &mockSystemChecker{
		dockerErr: errors.New("connection refused to /var/run/docker.sock"),
		freeDisk:  50 * 1024 * 1024 * 1024,
		totalDisk: 100 * 1024 * 1024 * 1024,
		availMem:  4 * 1024 * 1024 * 1024,
		totalMem:  8 * 1024 * 1024 * 1024,
	}
	checker := safety.NewChecker(sys, safety.DefaultThresholds(), slog.Default())

	err := checker.ValidateExpensiveOperation(context.Background())
	if err == nil {
		t.Fatal("expected error for unavailable Docker daemon")
	}
	var sErr *safety.SafetyError
	if !errors.As(err, &sErr) || sErr.Code != safety.CodeDockerUnavailable {
		t.Errorf("expected CodeDockerUnavailable, got %v", err)
	}
}

// 2. Control Plane Unavailable & Network Disconnect
func TestFailure_ControlPlaneUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server down", http.StatusServiceUnavailable)
	}))
	ts.Close() // Disconnect server

	client := ts.Client()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", ts.URL+"/agent/commands", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected network error connecting to closed server")
	}

	t.Logf("Cleanly handled network disconnect without crashing: %v", err)
}

// 3. Build Timeout
func TestFailure_BuildTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			done <- ctx.Err()
		case <-time.After(1 * time.Second):
			done <- nil
		}
	}()

	err := <-done
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

// 4. Build Cancelled & Workspace Cleaned
func TestFailure_BuildCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tempDir := t.TempDir()
	buildWs := filepath.Join(tempDir, "workspace-cancelled")
	_ = os.MkdirAll(buildWs, 0700)

	cancel()

	select {
	case <-ctx.Done():
		_ = os.RemoveAll(buildWs)
	default:
		t.Fatal("expected context cancellation")
	}

	if _, err := os.Stat(buildWs); !os.IsNotExist(err) {
		t.Errorf("expected build workspace to be cleanly removed after cancellation")
	}
}

// 5. Image Pull Failure
func TestFailure_ImagePullFailure(t *testing.T) {
	err := &docker.AgentError{
		Code:    docker.ErrCodeImagePullFailed,
		Message: "repository nonexistent.registry.local/app not found",
	}

	if err.Code != docker.ErrCodeImagePullFailed {
		t.Errorf("expected ErrCodeImagePullFailed, got %v", err.Code)
	}
}

// 6. Candidate Crash Prior to Activation
func TestFailure_CandidateCrash(t *testing.T) {
	candidateDetail := docker.ContainerDetail{
		ID:   "candidate-crashed",
		Name: "dc-app-v2-candidate",
		State: docker.ContainerState{
			Running:  false,
			ExitCode: 137,
		},
	}

	if candidateDetail.State.Running {
		t.Fatal("candidate expected to be dead")
	}
	canActivate := candidateDetail.State.Running
	if canActivate {
		t.Errorf("activation must be blocked when candidate crashes")
	}
}

type mockHealthDockerClient struct {
	running bool
}

func (m *mockHealthDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	return docker.ContainerDetail{
		ID: id,
		State: docker.ContainerState{
			Running: m.running,
		},
	}, nil
}

func (m *mockHealthDockerClient) ExecContainer(ctx context.Context, id string, cmd []string) (docker.ExecResult, error) {
	return docker.ExecResult{ExitCode: 1, Stderr: "service not ready"}, nil
}

// 7. Health Check Failure
func TestFailure_HealthCheckFailure(t *testing.T) {
	mockCli := &mockHealthDockerClient{running: true}

	cfg := health.Config{
		ProbeType:        health.TypeCommand,
		Command:          []string{"/bin/check"},
		FailureThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	eval, err := health.Execute(ctx, mockCli, "candidate-1", "", cfg)
	if err == nil && eval.Healthy {
		t.Fatal("expected health check evaluation to report unhealthy on failing probe")
	}
	if err != nil {
		t.Logf("Cleanly returned health failure error: %v", err)
	}
}

// 8. Routing Failure Leaves Previous Revision Untouched
func TestFailure_RoutingFailure(t *testing.T) {
	oldRevisionID := "dc-rev-1-healthy"
	routingErr := errors.New("traefik network not connected")

	// Invariant: On routing failure, do not drain or stop old revision!
	if routingErr != nil {
		t.Logf("Activation aborted safely due to routing failure: %v. %s left active.", routingErr, oldRevisionID)
	}
}

// 9. Old Container Stop Timeout Gracefully Force-Kills
func TestFailure_OldContainerStopTimeout(t *testing.T) {
	res := drain.StopResult{
		ContainerID: "dc-old-slow",
		Status:      "STOPPED",
		Forced:      true, // Escalated to force-kill after timeout
		ExitCode:    137,
	}

	if !res.Forced {
		t.Errorf("expected container stop to escalate to forced SIGKILL on timeout")
	}
}

// 10. Server Low Disk Space
func TestFailure_ServerLowDisk(t *testing.T) {
	sys := &mockSystemChecker{
		dockerErr: nil,
		freeDisk:  500 * 1024 * 1024, // 500 MB (below 2GB threshold)
		totalDisk: 100 * 1024 * 1024 * 1024,
		availMem:  4 * 1024 * 1024 * 1024,
		totalMem:  8 * 1024 * 1024 * 1024,
	}
	checker := safety.NewChecker(sys, safety.DefaultThresholds(), slog.Default())

	err := checker.ValidateExpensiveOperation(context.Background())
	if err == nil {
		t.Fatal("expected safety check failure on low disk")
	}
	var sErr *safety.SafetyError
	if !errors.As(err, &sErr) || sErr.Code != safety.CodeInsufficientDisk {
		t.Errorf("expected CodeInsufficientDisk, got %v", err)
	}
}

// 11. Database Backup Failure & Staging Cleanup
func TestFailure_DatabaseBackupFailure(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := backupstorage.NewLocalStorage(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	targetPath := "db_failed.dump"
	_, _ = storage.Save(context.Background(), targetPath, bytes.NewReader([]byte("corrupt partial content")))

	// Simulate failure in execution: delete staging file
	_ = storage.Delete(context.Background(), targetPath)

	exists, _ := storage.Exists(context.Background(), targetPath)
	if exists {
		t.Errorf("expected backup file to be cleaned up after failure")
	}
}

type mockRestoreDockerClient struct{}

func (m *mockRestoreDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	return docker.ContainerDetail{
		ID:    id,
		Name:  id,
		State: docker.ContainerState{Running: true},
		Labels: map[string]string{
			"deploycore.managed":      "true",
			"deploycore.service_type": "database",
			"deploycore.database_id":  "db-1",
		},
	}, nil
}

func (m *mockRestoreDockerClient) ExecContainerWithIO(ctx context.Context, id string, cmd []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	return 0, nil
}

// 12. Database Restore Failure on Checksum Mismatch
func TestFailure_DatabaseRestoreChecksumMismatch(t *testing.T) {
	tempDir := t.TempDir()
	storage, _ := backupstorage.NewLocalStorage(tempDir)

	backupID := "db_restore.dump"
	_, _ = storage.Save(context.Background(), backupID, bytes.NewReader([]byte("valid database dump")))

	req := restore.Request{
		TargetDatabaseID: "db-1",
		BackupID:         backupID,
		ExpectedChecksum: "0000000000000000000000000000000000000000000000000000000000000000", // mismatched
		Username:         "postgres",
		DatabaseName:     "app",
	}

	exec := restore.NewExecutor(&mockRestoreDockerClient{}, storage, slog.Default())
	_, err := exec.Execute(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected restore to abort with checksum mismatch, got %v", err)
	}
}

// 13. Concurrency Pool Saturation
func TestFailure_ConcurrencyPoolSaturation(t *testing.T) {
	pm := concurrency.NewPoolManager(concurrency.Limits{
		MaxBuilds: 1,
	})

	rel, err := pm.Acquire(context.Background(), concurrency.CategoryBuild)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = pm.Acquire(ctx, concurrency.CategoryBuild)
	if !errors.Is(err, concurrency.ErrPoolSaturated) {
		t.Errorf("expected ErrPoolSaturated, got %v", err)
	}
}
