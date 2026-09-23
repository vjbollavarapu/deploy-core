package rollback

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/appcontainer"
	"github.com/deploycore/deploy-core/apps/agent/internal/candidate"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/drain"
)

type mockClient struct {
	mu          sync.Mutex
	images      map[string]docker.ImageDetail
	containers  map[string]docker.ContainerDetail
	networks    map[string]docker.NetworkDetail
	connected   map[string][]string // net -> []containerID
	pulled      []string
	created     []string
	started     []string
	stopped     []string
	removed     []string
	healthFails bool
}

func newMockClient() *mockClient {
	return &mockClient{
		images:     make(map[string]docker.ImageDetail),
		containers: make(map[string]docker.ContainerDetail),
		networks:   make(map[string]docker.NetworkDetail),
		connected:  make(map[string][]string),
	}
}

func (m *mockClient) InspectImage(ctx context.Context, ref string) (docker.ImageDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img, ok := m.images[ref]
	if !ok {
		return docker.ImageDetail{}, errors.New("image not found")
	}
	return img, nil
}

func (m *mockClient) PullImageWithOptions(ctx context.Context, opts docker.PullImageOptions) (docker.PullImageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pulled = append(m.pulled, opts.Ref)
	img := docker.ImageDetail{
		ID:       "sha256:pulled-" + opts.Ref,
		RepoTags: []string{opts.Ref},
	}
	m.images[opts.Ref] = img
	return docker.PullImageResult{ImageID: img.ID, Status: "pulled"}, nil
}

func (m *mockClient) InspectNetwork(ctx context.Context, idOrName string) (docker.NetworkDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	net, ok := m.networks[idOrName]
	if !ok {
		return docker.NetworkDetail{ID: "net-" + idOrName, Name: idOrName}, nil
	}
	return net, nil
}

func (m *mockClient) CreateNetwork(ctx context.Context, req docker.CreateNetworkRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networks[req.Name] = docker.NetworkDetail{ID: "net-" + req.Name, Name: req.Name}
	return "net-" + req.Name, nil
}

func (m *mockClient) InspectVolume(ctx context.Context, name string) (docker.VolumeDetail, error) {
	return docker.VolumeDetail{Name: name}, nil
}

func (m *mockClient) CreateVolume(ctx context.Context, req docker.CreateVolumeRequest) (docker.VolumeSummary, error) {
	return docker.VolumeSummary{Name: req.Name}, nil
}

func (m *mockClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[id]
	if !ok {
		// Also lookup by Name
		for _, detail := range m.containers {
			if detail.Name == id || strings.TrimPrefix(detail.Name, "/") == strings.TrimPrefix(id, "/") {
				return detail, nil
			}
		}
		return docker.ContainerDetail{}, errors.New("container not found")
	}
	return c, nil
}

func (m *mockClient) CreateContainer(ctx context.Context, req docker.CreateContainerRequest) (docker.CreateContainerResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cid := "c-" + req.Name
	m.created = append(m.created, cid)
	m.containers[cid] = docker.ContainerDetail{
		ID:   cid,
		Name: req.Name,
		State: docker.ContainerState{
			Running: false,
		},
		Labels: req.Labels,
	}
	return docker.CreateContainerResult{ID: cid}, nil
}

func (m *mockClient) StartContainer(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, id)
	if c, ok := m.containers[id]; ok {
		c.State.Running = true
		if m.healthFails {
			c.State.Health = &docker.ContainerHealth{
				Status: "unhealthy",
			}
		} else {
			c.State.Health = &docker.ContainerHealth{
				Status: "healthy",
			}
		}
		m.containers[id] = c
	}
	return nil
}

func (m *mockClient) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, id)
	if c, ok := m.containers[id]; ok {
		c.State.Running = false
		m.containers[id] = c
	}
	return nil
}

func (m *mockClient) RemoveContainer(ctx context.Context, id string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, id)
	delete(m.containers, id)
	return nil
}

func (m *mockClient) ExecContainer(ctx context.Context, id string, cmd []string) (docker.ExecResult, error) {
	return docker.ExecResult{ExitCode: 0}, nil
}

func (m *mockClient) ConnectNetwork(ctx context.Context, networkID string, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.connected[networkID] {
		if c == containerID {
			return nil
		}
	}
	m.connected[networkID] = append(m.connected[networkID], containerID)
	return nil
}

func (m *mockClient) DisconnectNetwork(ctx context.Context, networkID string, containerID string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func TestExecutor_Execute_Success(t *testing.T) {
	cli := newMockClient()
	cli.images["app:v1"] = docker.ImageDetail{
		ID:       "sha256:app-v1-hash",
		RepoTags: []string{"app:v1"},
	}
	cli.containers["curr-active"] = docker.ContainerDetail{
		ID:   "curr-active",
		Name: "dc-app-r2-1",
		State: docker.ContainerState{
			Running: true,
		},
	}
	cli.connected["deploycore-proxy"] = []string{"curr-active"}

	executor := NewExecutor(cli, nil, nil, nil)

	spec := RollbackSpec{
		OrganizationID:     "org-1",
		ApplicationID:      "app-1",
		ApplicationSlug:    "app",
		DeploymentID:       "dep-rb-1",
		TargetRevisionID:   "rev-1",
		ReplicaIndex:       0,
		Instance:           1,
		Image:              "app:v1",
		CurrentContainerID: "curr-active",
		ProxyNetwork:       "deploycore-proxy",
		DrainDuration:      10 * time.Millisecond,
		TerminationTimeout: 10 * time.Second,
		RetentionPolicy:    drain.RetentionPolicyRetain,
		HealthPolicy: candidate.HealthPolicy{
			Type: candidate.HealthTypeContainerState,
		},
	}

	res, err := executor.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected rollback error: %v", err)
	}

	if res.Status != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %s", res.Status)
	}
	if res.TargetContainerID == "" {
		t.Errorf("expected target container ID to be populated")
	}
	if res.CurrentContainerStatus != "drained_and_stopped" {
		t.Errorf("expected current container status drained_and_stopped, got %s", res.CurrentContainerStatus)
	}

	// Verify target container was connected to proxy network
	targetConnected := false
	for _, id := range cli.connected["deploycore-proxy"] {
		if id == res.TargetContainerID {
			targetConnected = true
		}
		if id == "curr-active" {
			t.Errorf("current active container should be disconnected from proxy network")
		}
	}
	if !targetConnected {
		t.Errorf("target container must be connected to proxy network")
	}

	// Verify current active container was stopped but NOT removed under retain policy
	currStopped := false
	for _, id := range cli.stopped {
		if id == "curr-active" {
			currStopped = true
		}
	}
	if !currStopped {
		t.Errorf("expected curr-active container to be stopped")
	}
}

func TestExecutor_Execute_RebuildForbidden(t *testing.T) {
	cli := newMockClient()
	executor := NewExecutor(cli, nil, nil, nil)

	spec := RollbackSpec{
		ApplicationID:    "app-1",
		TargetRevisionID: "rev-1",
		Image:            "app:v1",
		Dockerfile:       "Dockerfile", // rebuild attempted
	}

	_, err := executor.Execute(context.Background(), spec)
	if !errors.Is(err, ErrRebuildForbidden) {
		t.Fatalf("expected ErrRebuildForbidden, got %v", err)
	}
}

func TestExecutor_Execute_TargetHealthFailure_CleansCandidateAndPreservesCurrent(t *testing.T) {
	cli := newMockClient()
	cli.images["app:v1"] = docker.ImageDetail{
		ID:       "sha256:app-v1-hash",
		RepoTags: []string{"app:v1"},
	}
	cli.containers["curr-active"] = docker.ContainerDetail{
		ID:   "curr-active",
		Name: "dc-app-r2-1",
		State: docker.ContainerState{
			Running: true,
		},
	}
	cli.connected["deploycore-proxy"] = []string{"curr-active"}

	// Configure mock so candidate health evaluation fails
	cli.healthFails = true

	executor := NewExecutor(cli, nil, nil, nil)

	spec := RollbackSpec{
		OrganizationID:     "org-1",
		ApplicationID:      "app-1",
		ApplicationSlug:    "app",
		DeploymentID:       "dep-rb-fail",
		TargetRevisionID:   "rev-1",
		ReplicaIndex:       0,
		Instance:           1,
		Image:              "app:v1",
		CurrentContainerID: "curr-active",
		ProxyNetwork:       "deploycore-proxy",
		HealthPolicy: candidate.HealthPolicy{
			Type: candidate.HealthTypeDocker,
		},
	}

	_, err := executor.Execute(context.Background(), spec)
	if !errors.Is(err, ErrTargetHealthFailed) {
		t.Fatalf("expected ErrTargetHealthFailed, got %v", err)
	}

	// Current active container must NOT have been touched or stopped!
	for _, id := range cli.stopped {
		if id == "curr-active" {
			t.Errorf("curr-active must NEVER be stopped on target failure")
		}
	}
	currInProxy := false
	for _, id := range cli.connected["deploycore-proxy"] {
		if id == "curr-active" {
			currInProxy = true
		}
	}
	if !currInProxy {
		t.Errorf("curr-active must remain connected to proxy network")
	}

	// Candidate container must have been cleaned up (stopped and removed)
	candName, _ := appcontainer.FormatName("app", "rev-1", 1)
	cleaned := false
	for _, id := range cli.removed {
		if id == "c-"+candName {
			cleaned = true
		}
	}
	if !cleaned {
		t.Errorf("failed candidate container must be cleaned up (removed)")
	}
}

func TestExecutor_Execute_ImagePullsByDigestWhenNotPresent(t *testing.T) {
	cli := newMockClient()
	// Image is not in cli.images initially
	cli.containers["curr-live"] = docker.ContainerDetail{
		ID:    "curr-live",
		State: docker.ContainerState{Running: true},
	}

	executor := NewExecutor(cli, nil, nil, nil)

	spec := RollbackSpec{
		ApplicationID:      "app-1",
		ApplicationSlug:    "app",
		DeploymentID:       "dep-pull",
		TargetRevisionID:   "rev-target",
		Image:              "registry.internal/app",
		ImageDigest:        "sha256:fedcba9876543210",
		CurrentContainerID: "curr-live",
		HealthPolicy: candidate.HealthPolicy{
			Type: candidate.HealthTypeContainerState,
		},
	}

	res, err := executor.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected rollback error: %v", err)
	}

	if res.Status != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", res.Status)
	}

	// Verify image was pulled by immutable digest
	expectedPullRef := "registry.internal/app@sha256:fedcba9876543210"
	if len(cli.pulled) != 1 || cli.pulled[0] != expectedPullRef {
		t.Errorf("expected pulled image %q, got %v", expectedPullRef, cli.pulled)
	}
}
