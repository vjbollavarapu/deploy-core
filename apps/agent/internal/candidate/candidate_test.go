package candidate

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/appcontainer"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/apps/agent/internal/volume"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// mockDockerClient implements Client for candidate testing.
type mockDockerClient struct {
	mu sync.Mutex

	images     map[string]docker.ImageDetail
	containers map[string]docker.ContainerDetail
	networks   map[string]docker.NetworkDetail
	volumes    map[string]docker.VolumeDetail

	createContainerFn  func(req docker.CreateContainerRequest) (docker.CreateContainerResult, error)
	startContainerFn   func(id string) error
	inspectContainerFn func(id string) (docker.ContainerDetail, error)
	pullImageFn        func(opts docker.PullImageOptions) (docker.PullImageResult, error)
	execContainerFn    func(id string, cmd []string) (docker.ExecResult, error)

	lastCreatedReq *docker.CreateContainerRequest
	startedIDs     []string
	stoppedIDs     []string
	removedIDs     []string
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		images:     make(map[string]docker.ImageDetail),
		containers: make(map[string]docker.ContainerDetail),
		networks:   make(map[string]docker.NetworkDetail),
		volumes:    make(map[string]docker.VolumeDetail),
	}
}

func (m *mockDockerClient) InspectImage(ctx context.Context, ref string) (docker.ImageDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if img, ok := m.images[ref]; ok {
		return img, nil
	}
	return docker.ImageDetail{}, errors.New("image not found")
}

func (m *mockDockerClient) PullImageWithOptions(ctx context.Context, opts docker.PullImageOptions) (docker.PullImageResult, error) {
	if m.pullImageFn != nil {
		return m.pullImageFn(opts)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	img := docker.ImageDetail{
		ID:       "sha256:pulledimage123",
		RepoTags: []string{opts.Ref},
		Size:     50 * 1024 * 1024,
	}
	m.images[opts.Ref] = img
	return docker.PullImageResult{
		Digest:  "sha256:pulledimage123",
		ImageID: "sha256:pulledimage123",
		Size:    img.Size,
		Status:  "pulled",
	}, nil
}

func (m *mockDockerClient) InspectNetwork(ctx context.Context, idOrName string) (docker.NetworkDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if net, ok := m.networks[idOrName]; ok {
		return net, nil
	}
	return docker.NetworkDetail{}, errors.New("network not found")
}

func (m *mockDockerClient) CreateNetwork(ctx context.Context, req docker.CreateNetworkRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	netDetail := docker.NetworkDetail{
		ID:     "net-" + req.Name,
		Name:   req.Name,
		Labels: req.Labels,
	}
	m.networks[req.Name] = netDetail
	return netDetail.ID, nil
}

func (m *mockDockerClient) InspectVolume(ctx context.Context, name string) (docker.VolumeDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if vol, ok := m.volumes[name]; ok {
		return vol, nil
	}
	return docker.VolumeDetail{}, errors.New("volume not found")
}

func (m *mockDockerClient) CreateVolume(ctx context.Context, req docker.CreateVolumeRequest) (docker.VolumeSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	volDetail := docker.VolumeDetail{
		Name:   req.Name,
		Labels: req.Labels,
	}
	m.volumes[req.Name] = volDetail
	return docker.VolumeSummary{Name: req.Name}, nil
}

func (m *mockDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	if m.inspectContainerFn != nil {
		return m.inspectContainerFn(id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.containers[id]; ok {
		return c, nil
	}
	return docker.ContainerDetail{}, errors.New("container not found")
}

func (m *mockDockerClient) CreateContainer(ctx context.Context, req docker.CreateContainerRequest) (docker.CreateContainerResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastCreatedReq = &req

	if m.createContainerFn != nil {
		return m.createContainerFn(req)
	}

	cid := "c-" + req.Name
	started := time.Now()
	detail := docker.ContainerDetail{
		ID:        cid,
		Name:      req.Name,
		Image:     req.Image,
		Labels:    req.PlatformLabels,
		StartedAt: &started,
		State: docker.ContainerState{
			Status:   "created",
			Running:  false,
			ExitCode: 0,
		},
		IPAddress: "10.0.1.5",
		Networks: map[string]string{
			"dc-daya-prod-private": "10.0.1.5",
		},
	}
	m.containers[cid] = detail
	m.containers[req.Name] = detail
	return docker.CreateContainerResult{ID: cid}, nil
}

func (m *mockDockerClient) StartContainer(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startedIDs = append(m.startedIDs, id)

	if m.startContainerFn != nil {
		return m.startContainerFn(id)
	}

	if c, ok := m.containers[id]; ok {
		c.State.Running = true
		c.State.Status = "running"
		now := time.Now()
		c.StartedAt = &now
		m.containers[id] = c
		if c.Name != "" {
			m.containers[c.Name] = c
		}
	}
	return nil
}

func (m *mockDockerClient) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stoppedIDs = append(m.stoppedIDs, id)
	if c, ok := m.containers[id]; ok {
		c.State.Running = false
		c.State.Status = "stopped"
		m.containers[id] = c
	}
	return nil
}

func (m *mockDockerClient) RemoveContainer(ctx context.Context, id string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removedIDs = append(m.removedIDs, id)
	delete(m.containers, id)
	return nil
}

func (m *mockDockerClient) ExecContainer(ctx context.Context, id string, cmd []string) (docker.ExecResult, error) {
	if m.execContainerFn != nil {
		return m.execContainerFn(id, cmd)
	}
	return docker.ExecResult{ExitCode: 0, Stdout: "OK"}, nil
}

func (m *mockDockerClient) ListNetworks(ctx context.Context) ([]docker.NetworkSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []docker.NetworkSummary
	for _, n := range m.networks {
		list = append(list, docker.NetworkSummary{
			ID:     n.ID,
			Name:   n.Name,
			Labels: n.Labels,
		})
	}
	return list, nil
}

func (m *mockDockerClient) RemoveNetwork(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.networks, id)
	return nil
}

func (m *mockDockerClient) ListVolumes(ctx context.Context) ([]docker.VolumeSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []docker.VolumeSummary
	for _, v := range m.volumes {
		list = append(list, docker.VolumeSummary{
			Name:   v.Name,
			Labels: v.Labels,
		})
	}
	return list, nil
}

func (m *mockDockerClient) RemoveVolume(ctx context.Context, name string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.volumes, name)
	return nil
}

func (m *mockDockerClient) ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []docker.ContainerSummary
	for _, c := range m.containers {
		list = append(list, docker.ContainerSummary{
			ID:     c.ID,
			Names:  []string{c.Name},
			Image:  c.Image,
			State:  c.State.Status,
			Labels: c.Labels,
		})
	}
	return list, nil
}

func validSpec() CandidateSpec {
	return CandidateSpec{
		Metadata: appcontainer.Metadata{
			OrganizationID: "org-123",
			ApplicationID:  "app-456",
			EnvironmentID:  "env-789",
			DeploymentID:   "dep-101",
			RevisionID:     "r49",
			Instance:       1,
			AppShortID:     "dayaapi",
		},
		Image:      "redis:7-alpine",
		PullPolicy: PullIfNotPresent,
		Networks: []NetworkSpec{
			{Name: "dc-dayaapi-env789-private"},
			{Name: protocol.ProxyNetworkName}, // Must be excluded during candidate phase!
		},
		Volumes: []VolumeSpec{
			{Name: "dc-app456-data", ContainerPath: "/data", ReadOnly: false},
		},
		Traefik: &docker.TraefikConfig{
			Enabled:     true,
			ServiceName: "dayaapi",
			Port:        8080,
			Host:        "api.daya.com",
		},
		HealthPolicy: HealthPolicy{
			Type:             HealthTypeContainerState,
			InitialDelay:     10 * time.Millisecond,
			Interval:         20 * time.Millisecond,
			SuccessThreshold: 1,
			FailureThreshold: 2,
		},
		StartupTimeout: 2 * time.Second,
	}
}

// TestStartCandidate_FullFlow verifies the entire 8-step flow succeeds and returns READY.
func TestStartCandidate_FullFlow(t *testing.T) {
	mock := newMockDockerClient()
	mock.images["redis:7-alpine"] = docker.ImageDetail{
		ID:       "sha256:existingimage",
		RepoTags: []string{"redis:7-alpine"},
	}

	netMgr := network.NewManager(mock)
	volMgr := volume.NewManager(mock)
	mgr := NewManager(mock, netMgr, volMgr, nil)

	spec := validSpec()
	res, err := mgr.StartCandidate(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected StartCandidate error: %v", err)
	}

	if res.Status != "READY" {
		t.Errorf("expected status READY, got %s", res.Status)
	}
	if res.ContainerName != "dc-dayaapi-r49-1" {
		t.Errorf("expected container name dc-dayaapi-r49-1, got %s", res.ContainerName)
	}
	if res.ImageID != "sha256:existingimage" {
		t.Errorf("expected image ID sha256:existingimage, got %s", res.ImageID)
	}
	if res.IPAddress != "10.0.1.5" {
		t.Errorf("expected IPAddress 10.0.1.5, got %s", res.IPAddress)
	}

	// Verify Step 4: Traffic isolation
	if mock.lastCreatedReq == nil {
		t.Fatal("expected CreateContainer to be called")
	}

	// 1. Candidate label must be set
	if mock.lastCreatedReq.PlatformLabels[protocol.LabelCandidate] != "true" {
		t.Errorf("expected deploycore.candidate=true label")
	}

	// 2. Traefik config may remain enabled (labels prepared); isolation is via network.
	if mock.lastCreatedReq.Traefik == nil || !mock.lastCreatedReq.Traefik.Enabled {
		t.Errorf("expected Traefik config to remain enabled on candidate (proxy network withheld)")
	}

	// 3. Proxy network must NOT be connected prematurely
	for _, n := range mock.lastCreatedReq.Networks {
		if n == protocol.ProxyNetworkName {
			t.Errorf("candidate container should NOT be connected to proxy network %s prematurely", protocol.ProxyNetworkName)
		}
	}

	// Verify Step 5: Start was called
	if len(mock.startedIDs) == 0 {
		t.Errorf("expected StartContainer to be called")
	}
}

// TestStartCandidate_PullImageWhenMissing verifies Step 1 pulls image when missing.
func TestStartCandidate_PullImageWhenMissing(t *testing.T) {
	mock := newMockDockerClient()
	pulled := false
	mock.pullImageFn = func(opts docker.PullImageOptions) (docker.PullImageResult, error) {
		pulled = true
		mock.images[opts.Ref] = docker.ImageDetail{ID: "sha256:newlypulled", RepoTags: []string{opts.Ref}}
		return docker.PullImageResult{ImageID: "sha256:newlypulled"}, nil
	}

	mgr := NewManager(mock, nil, nil, nil)
	spec := validSpec()
	res, err := mgr.StartCandidate(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pulled {
		t.Errorf("expected image to be pulled when not present")
	}
	if res.ImageID != "sha256:newlypulled" {
		t.Errorf("expected ImageID sha256:newlypulled, got %s", res.ImageID)
	}
}

// TestStartCandidate_StaleCandidateCleanup verifies that an existing container with
// deploycore.candidate="true" is cleaned up safely before recreation.
func TestStartCandidate_StaleCandidateCleanup(t *testing.T) {
	mock := newMockDockerClient()
	mock.images["redis:7-alpine"] = docker.ImageDetail{
		ID:       "sha256:img",
		RepoTags: []string{"redis:7-alpine"},
	}

	// Existing stale candidate container
	staleName := "dc-dayaapi-r49-1"
	mock.containers[staleName] = docker.ContainerDetail{
		ID:   "stale-123",
		Name: staleName,
		Labels: map[string]string{
			protocol.LabelManaged:   "true",
			protocol.LabelCandidate: "true",
		},
		State: docker.ContainerState{Running: false, Status: "exited"},
	}

	mgr := NewManager(mock, nil, nil, nil)
	res, err := mgr.StartCandidate(context.Background(), specWithHealth(HealthTypeContainerState))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "READY" {
		t.Errorf("expected status READY, got %s", res.Status)
	}

	// Verify old container was removed
	removed := false
	for _, id := range mock.removedIDs {
		if id == "stale-123" {
			removed = true
		}
	}
	if !removed {
		t.Errorf("expected stale candidate stale-123 to be removed")
	}
}

// TestStartCandidate_ActiveNonCandidateCollision verifies that if an active non-candidate
// container exists with the candidate name, creation fails to protect production.
func TestStartCandidate_ActiveNonCandidateCollision(t *testing.T) {
	mock := newMockDockerClient()
	mock.images["redis:7-alpine"] = docker.ImageDetail{
		ID:       "sha256:img",
		RepoTags: []string{"redis:7-alpine"},
	}

	// Active production container (IsCandidate = false)
	activeName := "dc-dayaapi-r49-1"
	mock.containers[activeName] = docker.ContainerDetail{
		ID:   "prod-active-123",
		Name: activeName,
		Labels: map[string]string{
			protocol.LabelManaged: "true",
			// NOT a candidate!
		},
		State: docker.ContainerState{Running: true, Status: "running"},
	}

	mgr := NewManager(mock, nil, nil, nil)
	_, err := mgr.StartCandidate(context.Background(), validSpec())
	if err == nil {
		t.Fatal("expected error due to collision with non-candidate container")
	}

	if !errors.Is(err, ErrStaleNonCandidate) && !strings.Contains(err.Error(), "active non-candidate") {
		t.Errorf("expected ErrStaleNonCandidate, got: %v", err)
	}
}

// TestStartCandidate_PrematureExitDetection verifies that if the container crashes on startup,
// the manager detects it immediately and returns ErrPrematureExit.
func TestStartCandidate_PrematureExitDetection(t *testing.T) {
	mock := newMockDockerClient()
	mock.images["redis:7-alpine"] = docker.ImageDetail{
		ID:       "sha256:img",
		RepoTags: []string{"redis:7-alpine"},
	}

	// Simulate container exiting immediately on start (e.g. exit code 137)
	mock.createContainerFn = func(req docker.CreateContainerRequest) (docker.CreateContainerResult, error) {
		cid := "c-" + req.Name
		detail := docker.ContainerDetail{
			ID:   cid,
			Name: req.Name,
			State: docker.ContainerState{
				Running:  false,
				Status:   "exited",
				ExitCode: 137,
				Error:    "out of memory",
			},
		}
		mock.containers[cid] = detail
		mock.containers[req.Name] = detail
		return docker.CreateContainerResult{ID: cid}, nil
	}

	mock.startContainerFn = func(id string) error {
		return nil
	}

	mgr := NewManager(mock, nil, nil, nil)
	_, err := mgr.StartCandidate(context.Background(), validSpec())
	if err == nil {
		t.Fatal("expected premature exit error")
	}

	if !errors.Is(err, ErrPrematureExit) && !strings.Contains(err.Error(), "exited") {
		t.Errorf("expected premature exit error, got: %v", err)
	}
}

// TestEvaluateHealth_DockerHealth verifies DOCKER health status evaluation.
func TestEvaluateHealth_DockerHealth(t *testing.T) {
	mock := newMockDockerClient()
	cid := "test-docker-hc"

	mock.containers[cid] = docker.ContainerDetail{
		ID: cid,
		State: docker.ContainerState{
			Running: true,
			Health: &docker.ContainerHealth{
				Status: "healthy",
			},
		},
	}

	policy := HealthPolicy{
		Type:             HealthTypeDocker,
		Interval:         10 * time.Millisecond,
		SuccessThreshold: 1,
		FailureThreshold: 2,
	}

	err := EvaluateHealth(context.Background(), mock, cid, "10.0.1.5", policy)
	if err != nil {
		t.Fatalf("expected healthy, got error: %v", err)
	}

	// Unhealthy case
	mock.containers[cid] = docker.ContainerDetail{
		ID: cid,
		State: docker.ContainerState{
			Running: true,
			Health: &docker.ContainerHealth{
				Status:        "unhealthy",
				FailingStreak: 3,
			},
		},
	}
	err = EvaluateHealth(context.Background(), mock, cid, "10.0.1.5", policy)
	if err == nil {
		t.Fatal("expected unhealthy error, got nil")
	}
}

// TestEvaluateHealth_HTTPProbe verifies HTTP probe checks against live test server.
func TestEvaluateHealth_HTTPProbe(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Extract port from ts.URL
	parts := strings.Split(ts.URL, ":")
	portStr := parts[len(parts)-1]
	port, _ := strconv.Atoi(portStr)

	mock := newMockDockerClient()
	mock.containers["c1"] = docker.ContainerDetail{
		ID:    "c1",
		State: docker.ContainerState{Running: true},
	}

	policy := HealthPolicy{
		Type:             HealthTypeHTTP,
		HTTPPort:         port,
		HTTPPath:         "/healthz",
		HTTPScheme:       "http",
		ExpectedStatus:   200,
		Interval:         10 * time.Millisecond,
		SuccessThreshold: 1,
		FailureThreshold: 2,
	}

	err := EvaluateHealth(context.Background(), mock, "c1", "127.0.0.1", policy)
	if err != nil {
		t.Fatalf("expected http probe to succeed, got %v", err)
	}

	// Test failing endpoint
	policy.HTTPPath = "/notfound"
	err = EvaluateHealth(context.Background(), mock, "c1", "127.0.0.1", policy)
	if err == nil {
		t.Fatal("expected http probe failure on 404, got nil")
	}
}

// TestEvaluateHealth_TCPProbe verifies TCP socket dial probe.
func TestEvaluateHealth_TCPProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)

	mock := newMockDockerClient()
	policy := HealthPolicy{
		Type:             HealthTypeTCP,
		TCPPort:          addr.Port,
		Interval:         10 * time.Millisecond,
		SuccessThreshold: 1,
		FailureThreshold: 2,
	}

	err = EvaluateHealth(context.Background(), mock, "c1", "127.0.0.1", policy)
	if err != nil {
		t.Fatalf("expected tcp probe success, got: %v", err)
	}
}

// TestEvaluateHealth_CommandProbe verifies in-container command exec probe.
func TestEvaluateHealth_CommandProbe(t *testing.T) {
	mock := newMockDockerClient()
	mock.execContainerFn = func(id string, cmd []string) (docker.ExecResult, error) {
		if len(cmd) > 0 && cmd[0] == "redis-cli" {
			return docker.ExecResult{ExitCode: 0, Stdout: "PONG"}, nil
		}
		return docker.ExecResult{ExitCode: 1, Stderr: "command failed"}, nil
	}

	policy := HealthPolicy{
		Type:             HealthTypeCommand,
		Command:          []string{"redis-cli", "ping"},
		Interval:         10 * time.Millisecond,
		SuccessThreshold: 1,
		FailureThreshold: 2,
	}

	err := EvaluateHealth(context.Background(), mock, "c1", "10.0.1.5", policy)
	if err != nil {
		t.Fatalf("expected command probe success, got: %v", err)
	}

	policy.Command = []string{"fail-cmd"}
	err = EvaluateHealth(context.Background(), mock, "c1", "10.0.1.5", policy)
	if err == nil {
		t.Fatal("expected command probe failure, got nil")
	}
}

func specWithHealth(ht HealthCheckType) CandidateSpec {
	s := validSpec()
	s.HealthPolicy.Type = ht
	return s
}
