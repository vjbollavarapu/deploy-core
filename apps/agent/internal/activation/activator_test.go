package activation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

type mockDockerClient struct {
	mu            sync.Mutex
	containers    map[string]docker.ContainerDetail
	connected     map[string][]string // net -> []containerID
	stopped       []string
	removed       []string
	stopTimeout   time.Duration
	connectErr    error
	disconnectErr error
	stopErr       error
	removeErr     error
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		containers: make(map[string]docker.ContainerDetail),
		connected:  make(map[string][]string),
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

func (m *mockDockerClient) ConnectNetwork(ctx context.Context, networkID string, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connectErr != nil {
		return m.connectErr
	}
	for _, c := range m.connected[networkID] {
		if c == containerID {
			return nil // idempotent
		}
	}
	m.connected[networkID] = append(m.connected[networkID], containerID)
	return nil
}

func (m *mockDockerClient) DisconnectNetwork(ctx context.Context, networkID string, containerID string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disconnectErr != nil {
		return m.disconnectErr
	}
	list := m.connected[networkID]
	newList := make([]string, 0, len(list))
	for _, c := range list {
		if c != containerID {
			newList = append(newList, c)
		}
	}
	m.connected[networkID] = newList
	return nil
}

func (m *mockDockerClient) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = append(m.stopped, id)
	m.stopTimeout = timeout
	return nil
}

func (m *mockDockerClient) RemoveContainer(ctx context.Context, id string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.removeErr != nil {
		return m.removeErr
	}
	m.removed = append(m.removed, id)
	return nil
}

type mockProber struct {
	probeErr error
	called   bool
}

func (p *mockProber) Probe(ctx context.Context, url string, timeout time.Duration, expectedStatus int) error {
	p.called = true
	return p.probeErr
}

func TestActivator_Activate_NormalZeroDowntime(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cand-123"] = docker.ContainerDetail{
		ID:   "cand-123",
		Name: "dc-app-r2-1",
		State: docker.ContainerState{
			Running: true,
		},
	}
	client.containers["old-100"] = docker.ContainerDetail{
		ID:   "old-100",
		Name: "dc-app-r1-1",
		State: docker.ContainerState{
			Running: true,
		},
	}
	client.connected["deploycore-proxy"] = []string{"old-100"}

	prober := &mockProber{}
	activator := NewActivator(client, prober, nil)

	spec := ActivationSpec{
		CandidateContainerID: "cand-123",
		OldContainerID:       "old-100",
		ProxyNetwork:         "deploycore-proxy",
		VerifyRoute:          true,
		RouteVerifyURL:       "http://app.internal/health",
		DrainDuration:        10 * time.Millisecond,
		StopTimeout:          10 * time.Second,
		RetentionPolicy:      RetentionPolicyRetain,
	}

	result, err := activator.Activate(context.Background(), spec)
	if err != nil {
		t.Fatalf("expected activate to succeed, got %v", err)
	}

	if result.Status != "ACTIVATED" {
		t.Errorf("expected status ACTIVATED, got %s", result.Status)
	}
	if result.OldContainerStatus != "drained_and_stopped" {
		t.Errorf("expected old status drained_and_stopped, got %s", result.OldContainerStatus)
	}
	if !prober.called {
		t.Errorf("expected route prober to be called")
	}

	// Verify candidate is connected to proxy network
	candConnected := false
	for _, c := range client.connected["deploycore-proxy"] {
		if c == "cand-123" {
			candConnected = true
		}
		if c == "old-100" {
			t.Errorf("old container should have been disconnected from proxy network")
		}
	}
	if !candConnected {
		t.Errorf("candidate container should be connected to proxy network")
	}

	// Verify old container was stopped but NOT removed (retention = retain)
	if len(client.stopped) != 1 || client.stopped[0] != "old-100" {
		t.Errorf("expected old-100 to be stopped, got %v", client.stopped)
	}
	if len(client.removed) != 0 {
		t.Errorf("expected 0 removed containers under retain policy, got %v", client.removed)
	}
}

func TestActivator_Activate_InitialDeploy_NoOldContainer(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cand-1"] = docker.ContainerDetail{
		ID:   "cand-1",
		Name: "dc-first-r1-1",
		State: docker.ContainerState{
			Running: true,
		},
	}

	activator := NewActivator(client, nil, nil)
	spec := ActivationSpec{
		CandidateContainerID: "cand-1",
		ProxyNetwork:         "deploycore-proxy",
	}

	result, err := activator.Activate(context.Background(), spec)
	if err != nil {
		t.Fatalf("expected activate to succeed, got %v", err)
	}

	if result.OldContainerStatus != "none" {
		t.Errorf("expected old container status none, got %s", result.OldContainerStatus)
	}
	if len(client.stopped) != 0 {
		t.Errorf("expected 0 stopped containers, got %v", client.stopped)
	}
}

func TestActivator_Activate_CandidateNotRunning(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cand-dead"] = docker.ContainerDetail{
		ID: "cand-dead",
		State: docker.ContainerState{
			Running: false,
		},
	}

	activator := NewActivator(client, nil, nil)
	spec := ActivationSpec{
		CandidateContainerID: "cand-dead",
		OldContainerID:       "old-live",
	}

	_, err := activator.Activate(context.Background(), spec)
	if !errors.Is(err, ErrCandidateNotRunning) {
		t.Fatalf("expected ErrCandidateNotRunning, got %v", err)
	}

	// Old container must be untouched
	if len(client.stopped) != 0 {
		t.Errorf("expected old container not stopped")
	}
}

func TestActivator_Activate_CandidateUnhealthy(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cand-bad"] = docker.ContainerDetail{
		ID: "cand-bad",
		State: docker.ContainerState{
			Running: true,
			Health: &docker.ContainerHealth{
				Status: "unhealthy",
			},
		},
	}

	activator := NewActivator(client, nil, nil)
	spec := ActivationSpec{
		CandidateContainerID: "cand-bad",
	}

	_, err := activator.Activate(context.Background(), spec)
	if !errors.Is(err, ErrCandidateUnhealthy) {
		t.Fatalf("expected ErrCandidateUnhealthy, got %v", err)
	}
}

func TestActivator_Activate_RouteVerificationFailure_RollsBack(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cand-fail"] = docker.ContainerDetail{
		ID:   "cand-fail",
		Name: "dc-app-cand",
		State: docker.ContainerState{
			Running: true,
		},
	}
	client.connected["deploycore-proxy"] = []string{"old-serving"}

	prober := &mockProber{probeErr: errors.New("connection refused")}
	activator := NewActivator(client, prober, nil)

	spec := ActivationSpec{
		CandidateContainerID: "cand-fail",
		OldContainerID:       "old-serving",
		ProxyNetwork:         "deploycore-proxy",
		VerifyRoute:          true,
		RouteVerifyURL:       "http://probe.invalid",
	}

	_, err := activator.Activate(context.Background(), spec)
	if !errors.Is(err, ErrRouteVerificationFailed) {
		t.Fatalf("expected ErrRouteVerificationFailed, got %v", err)
	}

	// Candidate must have been disconnected from proxy network (rollback)
	for _, c := range client.connected["deploycore-proxy"] {
		if c == "cand-fail" {
			t.Errorf("candidate should have been rolled back and disconnected from proxy")
		}
	}

	// Old serving container must still be intact and never stopped
	if len(client.stopped) != 0 {
		t.Errorf("old container must NOT have been stopped on verification failure")
	}
	oldInProxy := false
	for _, c := range client.connected["deploycore-proxy"] {
		if c == "old-serving" {
			oldInProxy = true
		}
	}
	if !oldInProxy {
		t.Errorf("old container must remain connected to proxy network")
	}
}

func TestActivator_Activate_RetentionPolicyRemove(t *testing.T) {
	client := newMockDockerClient()
	client.containers["cand-1"] = docker.ContainerDetail{
		ID:    "cand-1",
		State: docker.ContainerState{Running: true},
	}
	client.containers["old-1"] = docker.ContainerDetail{
		ID:    "old-1",
		State: docker.ContainerState{Running: true},
	}

	activator := NewActivator(client, nil, nil)
	spec := ActivationSpec{
		CandidateContainerID: "cand-1",
		OldContainerID:       "old-1",
		RetentionPolicy:      RetentionPolicyRemove,
	}

	result, err := activator.Activate(context.Background(), spec)
	if err != nil {
		t.Fatalf("expected activation success: %v", err)
	}

	if result.OldContainerStatus != "drained_and_removed" {
		t.Errorf("expected drained_and_removed, got %s", result.OldContainerStatus)
	}
	if len(client.removed) != 1 || client.removed[0] != "old-1" {
		t.Errorf("expected old-1 to be removed, got %v", client.removed)
	}
}

func TestDefaultRouteProber_SuccessAndFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer ts.Close()

	prober := &DefaultRouteProber{Client: ts.Client()}

	if err := prober.Probe(context.Background(), ts.URL+"/ok", time.Second, 200); err != nil {
		t.Errorf("expected success on /ok, got %v", err)
	}

	if err := prober.Probe(context.Background(), ts.URL+"/bad", time.Second, 200); err == nil {
		t.Errorf("expected error on /bad 503 status")
	}
}
