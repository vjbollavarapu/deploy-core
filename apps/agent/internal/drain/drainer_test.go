package drain

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

type mockDockerClient struct {
	mu           sync.Mutex
	containers   map[string]docker.ContainerDetail
	disconnected map[string][]string // net -> []containerID
	stopped      map[string]time.Duration
	removed      []string
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		containers:   make(map[string]docker.ContainerDetail),
		disconnected: make(map[string][]string),
		stopped:      make(map[string]time.Duration),
	}
}

func (m *mockDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.containers[id]
	if !ok {
		return docker.ContainerDetail{}, errors.New("container not found")
	}
	return d, nil
}

func (m *mockDockerClient) DisconnectNetwork(ctx context.Context, networkID string, containerID string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnected[networkID] = append(m.disconnected[networkID], containerID)
	return nil
}

func (m *mockDockerClient) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped[id] = timeout
	// Update container state to stopped
	if d, ok := m.containers[id]; ok {
		d.State.Running = false
		d.State.ExitCode = 143 // SIGTERM exit code
		m.containers[id] = d
	}
	return nil
}

func (m *mockDockerClient) RemoveContainer(ctx context.Context, id string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, id)
	delete(m.containers, id)
	return nil
}

func TestDrainer_Stop_GracefulWithRoutingDrain(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cont-1"] = docker.ContainerDetail{
		ID:   "cont-1",
		Name: "dc-web-r1-1",
		State: docker.ContainerState{
			Running:  true,
			ExitCode: 0,
		},
		Networks: map[string]string{
			"deploycore-proxy": "172.20.0.5",
		},
	}

	drainer := NewDrainer(client, nil)

	spec := StopSpec{
		ContainerID:        "cont-1",
		DrainRouting:       true,
		ProxyNetwork:       "deploycore-proxy",
		DrainDuration:      10 * time.Millisecond,
		TerminationTimeout: 10 * time.Second,
		ForceKill:          false,
		RetentionPolicy:    RetentionPolicyRetain,
	}

	res, err := drainer.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if res.Status != "STOPPED" {
		t.Errorf("expected status STOPPED, got %s", res.Status)
	}
	if !res.Drained {
		t.Errorf("expected drained to be true")
	}
	if res.Forced {
		t.Errorf("expected forced to be false")
	}
	if res.ExitCode != 143 {
		t.Errorf("expected exit code 143, got %d", res.ExitCode)
	}

	// Verify disconnect from proxy network
	disconnected := false
	for _, id := range client.disconnected["deploycore-proxy"] {
		if id == "cont-1" {
			disconnected = true
		}
	}
	if !disconnected {
		t.Errorf("expected cont-1 to be disconnected from deploycore-proxy")
	}

	// Verify stop timeout matches termination timeout
	if timeout, ok := client.stopped["cont-1"]; !ok || timeout != 10*time.Second {
		t.Errorf("expected stop timeout 10s, got %v", timeout)
	}

	// Verify not removed under retain
	if len(client.removed) != 0 {
		t.Errorf("expected 0 removed containers, got %v", client.removed)
	}
}

func TestDrainer_Stop_NonTrafficService(t *testing.T) {
	client := newMockDockerClient()
	client.containers["worker-1"] = docker.ContainerDetail{
		ID:   "worker-1",
		Name: "dc-worker-r1-1",
		State: docker.ContainerState{
			Running: true,
		},
		Networks: map[string]string{
			"app-private": "10.0.0.2",
		},
	}

	drainer := NewDrainer(client, nil)

	spec := StopSpec{
		ContainerID:        "worker-1",
		DrainRouting:       false,
		TerminationTimeout: 5 * time.Second,
	}

	res, err := drainer.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if res.Drained {
		t.Errorf("expected drained to be false for non-traffic service")
	}
	if len(client.disconnected["deploycore-proxy"]) != 0 {
		t.Errorf("expected no network disconnect calls")
	}
}

func TestDrainer_Stop_IdempotentAlreadyStopped(t *testing.T) {
	client := newMockDockerClient()
	client.containers["stopped-1"] = docker.ContainerDetail{
		ID:   "stopped-1",
		Name: "dc-stopped-1",
		State: docker.ContainerState{
			Running:  false,
			ExitCode: 0,
		},
	}

	drainer := NewDrainer(client, nil)

	spec := StopSpec{
		ContainerID: "stopped-1",
	}

	res, err := drainer.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if res.Status != "ALREADY_STOPPED" {
		t.Errorf("expected ALREADY_STOPPED, got %s", res.Status)
	}
	if _, stopped := client.stopped["stopped-1"]; stopped {
		t.Errorf("expected StopContainer not to be called on already stopped container")
	}
}

func TestDrainer_Stop_ExplicitForceKill(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cont-force"] = docker.ContainerDetail{
		ID:   "cont-force",
		Name: "dc-stuck",
		State: docker.ContainerState{
			Running: true,
		},
	}

	drainer := NewDrainer(client, nil)

	spec := StopSpec{
		ContainerID: "cont-force",
		ForceKill:   true,
	}

	res, err := drainer.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if !res.Forced {
		t.Errorf("expected forced to be true")
	}
	if client.stopped["cont-force"] != 0 {
		t.Errorf("expected stop timeout 0 for force kill, got %v", client.stopped["cont-force"])
	}
}

func TestDrainer_Stop_RetentionPolicyRemove(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cont-rm"] = docker.ContainerDetail{
		ID:   "cont-rm",
		Name: "dc-ephemeral",
		State: docker.ContainerState{
			Running: true,
		},
	}

	drainer := NewDrainer(client, nil)

	spec := StopSpec{
		ContainerID:     "cont-rm",
		RetentionPolicy: RetentionPolicyRemove,
	}

	res, err := drainer.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if res.Status != "REMOVED" {
		t.Errorf("expected status REMOVED, got %s", res.Status)
	}
	if len(client.removed) != 1 || client.removed[0] != "cont-rm" {
		t.Errorf("expected cont-rm to be removed, got %v", client.removed)
	}
}

func TestDrainer_Stop_DrainContextCancelled(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cont-ctx"] = docker.ContainerDetail{
		ID:       "cont-ctx",
		State:    docker.ContainerState{Running: true},
		Networks: map[string]string{"deploycore-proxy": "172.20.0.9"},
	}

	drainer := NewDrainer(client, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel context immediately

	spec := StopSpec{
		ContainerID:   "cont-ctx",
		DrainRouting:  true,
		DrainDuration: 1 * time.Second,
	}

	_, err := drainer.Stop(ctx, spec)
	if !errors.Is(err, ErrDrainCancelled) {
		t.Fatalf("expected ErrDrainCancelled, got %v", err)
	}
}
