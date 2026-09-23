package docker_test

import (
	"context"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// Integration tests that skip gracefully when Docker is not available.
// These tests verify the operation layer against a live daemon to
// confirm mapping and type safety — they are not unit tests.

func newTestClient(t *testing.T) *docker.Client {
	t.Helper()
	cfg := config.Config{}
	c, err := docker.NewClient(cfg)
	if err != nil {
		t.Skipf("skipping: cannot create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.CheckConnectivity(ctx); err != nil {
		t.Skipf("skipping: docker not reachable: %v", err)
	}
	return c
}

func TestPing(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping error: %v", err)
	}
	if res.APIVersion == "" {
		t.Error("expected non-empty APIVersion")
	}
}

func TestInfo(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("Info error: %v", err)
	}
	if info.ServerVersion == "" {
		t.Error("expected non-empty ServerVersion")
	}
}

func TestVersion(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	v, err := c.Version(ctx)
	if err != nil {
		t.Fatalf("Version error: %v", err)
	}
	if v.Version == "" {
		t.Error("expected non-empty Version")
	}
}

func TestListContainers(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Just verify it returns without error; the actual count depends on the environment.
	_, err := c.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers error: %v", err)
	}
}

func TestListImages(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.ListImages(ctx, false)
	if err != nil {
		t.Fatalf("ListImages error: %v", err)
	}
}

func TestListNetworks(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nets, err := c.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks error: %v", err)
	}
	// Expect at least the default bridge network
	if len(nets) == 0 {
		t.Error("expected at least one network (bridge)")
	}
}

func TestListVolumes(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.ListVolumes(ctx)
	if err != nil {
		t.Fatalf("ListVolumes error: %v", err)
	}
}

func TestGetSystemMetrics(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m, err := c.GetSystemMetrics(ctx)
	if err != nil {
		t.Fatalf("GetSystemMetrics error: %v", err)
	}
	if m.Hostname == "" {
		t.Error("expected non-empty Hostname")
	}
}

func TestGetDockerMetrics(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m, err := c.GetDockerMetrics(ctx)
	if err != nil {
		t.Fatalf("GetDockerMetrics error: %v", err)
	}
	if m.Version == "" {
		t.Error("expected non-empty DockerMetrics.Version")
	}
}

func TestAgentError_NotFound(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Inspecting a nonexistent container should return a stable AgentError.
	_, err := c.InspectContainer(ctx, "definitely-does-not-exist-xyz-"+t.Name())
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
	ae, ok := err.(*docker.AgentError)
	if !ok {
		t.Fatalf("expected *docker.AgentError, got %T: %v", err, err)
	}
	if ae.Code != docker.ErrCodeNotFound {
		t.Errorf("expected ErrCodeNotFound, got %s", ae.Code)
	}
}

func TestDockerEvents_CancelImmediately(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	cancel() // cancel immediately

	evtCh := make(chan docker.Event, 4)
	errCh := make(chan error, 1)
	c.DockerEvents(ctx, evtCh, errCh)

	// Channel must eventually close
	select {
	case <-time.After(3 * time.Second):
		t.Fatal("DockerEvents channel did not close after context cancel")
	case _, _ = <-evtCh:
	}
}
