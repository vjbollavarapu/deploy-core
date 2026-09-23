package health

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

type mockHealthClient struct {
	containers map[string]docker.ContainerDetail
	execFn     func(id string, cmd []string) (docker.ExecResult, error)
}

func (m *mockHealthClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	if c, ok := m.containers[id]; ok {
		return c, nil
	}
	return docker.ContainerDetail{}, errors.New("container not found")
}

func (m *mockHealthClient) ExecContainer(ctx context.Context, id string, cmd []string) (docker.ExecResult, error) {
	if m.execFn != nil {
		return m.execFn(id, cmd)
	}
	return docker.ExecResult{ExitCode: 0, Stdout: "OK"}, nil
}

// TestHTTPProbe_ExactAndRangeCodes verifies HTTP probe status code evaluation.
func TestHTTPProbe_ExactAndRangeCodes(t *testing.T) {
	statusCode := http.StatusOK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(statusCode)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	parts := strings.Split(ts.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	ctx := context.Background()

	// 1. Exact expected code 200
	cfg := Config{
		ProbeType:      TypeHTTP,
		HTTPPort:       port,
		HTTPPath:       "/health",
		ExpectedStatus: 200,
	}
	obs := probeHTTP(ctx, "127.0.0.1", cfg)
	if !obs.Success || obs.HTTPStatusCode != 200 {
		t.Fatalf("expected HTTP 200 success, got %+v", obs)
	}

	// 2. Expected range e.g. "200-299" with 204 No Content
	statusCode = http.StatusNoContent
	cfg = Config{
		ProbeType:     TypeHTTP,
		HTTPPort:      port,
		HTTPPath:      "/health",
		ExpectedRange: "200-299",
	}
	obs = probeHTTP(ctx, "127.0.0.1", cfg)
	if !obs.Success || obs.HTTPStatusCode != 204 {
		t.Fatalf("expected HTTP 204 success within range 200-299, got %+v", obs)
	}

	// 3. Status not in range e.g. 500
	statusCode = http.StatusInternalServerError
	obs = probeHTTP(ctx, "127.0.0.1", cfg)
	if obs.Success {
		t.Fatalf("expected HTTP 500 to fail, got success")
	}

	// 4. Comma-separated range "200,201" with 204 -> fail
	cfg.ExpectedRange = "200,201"
	obs = probeHTTP(ctx, "127.0.0.1", cfg)
	if obs.Success {
		t.Fatalf("expected HTTP 500 to fail on comma range, got success")
	}
}

// TestTCPProbe verifies socket connection checks.
func TestTCPProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	ctx := context.Background()

	// Open port
	obs := probeTCP(ctx, "127.0.0.1", port)
	if !obs.Success {
		t.Fatalf("expected open TCP port to succeed, got %+v", obs)
	}

	// Closed port
	obs = probeTCP(ctx, "127.0.0.1", port+9999)
	if obs.Success {
		t.Fatalf("expected closed TCP port to fail, got success")
	}
}

// TestCommandProbe verifies that command probe executes via DockerClient.ExecContainer.
func TestCommandProbe(t *testing.T) {
	executedInsideContainer := false
	mock := &mockHealthClient{
		execFn: func(id string, cmd []string) (docker.ExecResult, error) {
			executedInsideContainer = true
			if len(cmd) > 0 && cmd[0] == "pg_isready" {
				return docker.ExecResult{ExitCode: 0, Stdout: "accepting connections"}, nil
			}
			return docker.ExecResult{ExitCode: 2, Stderr: "connection refused"}, nil
		},
	}

	ctx := context.Background()

	// 1. Successful command probe
	obs := probeCommand(ctx, mock, "c-123", []string{"pg_isready", "-h", "localhost"})
	if !obs.Success || obs.ExitCode != 0 {
		t.Fatalf("expected successful command probe, got %+v", obs)
	}
	if !executedInsideContainer {
		t.Fatal("expected command to execute via DockerClient.ExecContainer")
	}

	// 2. Failing command probe
	obs = probeCommand(ctx, mock, "c-123", []string{"failing_command"})
	if obs.Success || obs.ExitCode != 2 {
		t.Fatalf("expected failing command probe with exit code 2, got %+v", obs)
	}

	// 3. Empty command rejection
	obs = probeCommand(ctx, mock, "c-123", nil)
	if obs.Success || !strings.Contains(obs.Error, "empty") {
		t.Fatalf("expected empty command rejection, got %+v", obs)
	}
}

// TestContainerStateProbe verifies container running and Docker native health evaluations.
func TestContainerStateProbe(t *testing.T) {
	mock := &mockHealthClient{
		containers: map[string]docker.ContainerDetail{
			"running-healthy": {
				ID: "running-healthy",
				State: docker.ContainerState{
					Running: true,
					Pid:     1234,
				},
			},
			"running-docker-healthy": {
				ID: "running-docker-healthy",
				State: docker.ContainerState{
					Running: true,
					Health: &docker.ContainerHealth{
						Status: "healthy",
					},
				},
			},
			"running-docker-unhealthy": {
				ID: "running-docker-unhealthy",
				State: docker.ContainerState{
					Running: true,
					Health: &docker.ContainerHealth{
						Status:        "unhealthy",
						FailingStreak: 3,
					},
				},
			},
			"dead-container": {
				ID: "dead-container",
				State: docker.ContainerState{
					Dead: true,
				},
			},
			"exited-container": {
				ID: "exited-container",
				State: docker.ContainerState{
					Running:  false,
					Status:   "exited",
					ExitCode: 1,
				},
			},
		},
	}

	ctx := context.Background()

	// 1. Running container without Docker healthcheck
	obs := probeContainerState(ctx, mock, "running-healthy")
	if !obs.Success {
		t.Fatalf("expected running container success, got %+v", obs)
	}

	// 2. Docker native healthy
	obs = probeContainerState(ctx, mock, "running-docker-healthy")
	if !obs.Success {
		t.Fatalf("expected docker healthy success, got %+v", obs)
	}

	// 3. Docker native unhealthy
	obs = probeContainerState(ctx, mock, "running-docker-unhealthy")
	if obs.Success {
		t.Fatalf("expected docker unhealthy failure, got success")
	}

	// 4. Dead container
	obs = probeContainerState(ctx, mock, "dead-container")
	if obs.Success {
		t.Fatalf("expected dead container failure, got success")
	}

	// 5. Exited container
	obs = probeContainerState(ctx, mock, "exited-container")
	if obs.Success {
		t.Fatalf("expected exited container failure, got success")
	}
}

// TestExecute_FullLifecycleThresholds verifies the complete health check execution flow.
func TestExecute_FullLifecycleThresholds(t *testing.T) {
	mock := &mockHealthClient{
		containers: map[string]docker.ContainerDetail{
			"c-app": {
				ID: "c-app",
				State: docker.ContainerState{
					Running: true,
					Pid:     5678,
				},
			},
		},
	}

	cfg := Config{
		ProbeType:        TypeContainerState,
		InitialDelay:     5 * time.Millisecond,
		Interval:         10 * time.Millisecond,
		Timeout:          500 * time.Millisecond,
		SuccessThreshold: 2,
		FailureThreshold: 2,
	}

	res, err := Execute(context.Background(), mock, "c-app", "10.0.0.2", cfg)
	if err != nil {
		t.Fatalf("unexpected Execute error: %v", err)
	}

	if res.Status != StateHealthy || !res.Healthy {
		t.Errorf("expected Status HEALTHY, got %s", res.Status)
	}
	if res.ConsecutiveSuccesses < 2 {
		t.Errorf("expected at least 2 consecutive successes, got %d", res.ConsecutiveSuccesses)
	}
	if len(res.Observations) < 2 {
		t.Errorf("expected at least 2 observations recorded, got %d", len(res.Observations))
	}
}

// TestExecute_FailureThreshold verifies failure threshold triggers Unhealthy.
func TestExecute_FailureThreshold(t *testing.T) {
	mock := &mockHealthClient{
		containers: map[string]docker.ContainerDetail{
			"c-bad": {
				ID: "c-bad",
				State: docker.ContainerState{
					Running:  false,
					Status:   "exited",
					ExitCode: 137,
				},
			},
		},
	}

	cfg := Config{
		ProbeType:        TypeContainerState,
		InitialDelay:     5 * time.Millisecond,
		Interval:         10 * time.Millisecond,
		Timeout:          500 * time.Millisecond,
		SuccessThreshold: 1,
		FailureThreshold: 2,
	}

	res, err := Execute(context.Background(), mock, "c-bad", "10.0.0.2", cfg)
	if err == nil {
		t.Fatal("expected error when failure threshold reached")
	}

	if res.Status != StateUnhealthy || res.Healthy {
		t.Errorf("expected Status UNHEALTHY, got %s", res.Status)
	}
	if res.ConsecutiveFailures != 2 {
		t.Errorf("expected consecutive failures = 2, got %d", res.ConsecutiveFailures)
	}
}
