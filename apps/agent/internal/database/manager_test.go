package database

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockDatabaseDockerClient struct {
	containers map[string]docker.ContainerDetail
	volumes    map[string]docker.VolumeDetail
	networks   map[string]docker.NetworkDetail
	images     map[string]docker.ImageDetail

	createdContainerReq *docker.CreateContainerRequest
	startedContainers   []string
	stoppedContainers   []string
}

func newMockDocker() *mockDatabaseDockerClient {
	return &mockDatabaseDockerClient{
		containers: make(map[string]docker.ContainerDetail),
		volumes:    make(map[string]docker.VolumeDetail),
		networks:   make(map[string]docker.NetworkDetail),
		images:     make(map[string]docker.ImageDetail),
	}
}

func (m *mockDatabaseDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	d, ok := m.containers[id]
	if !ok {
		return docker.ContainerDetail{}, fmt.Errorf("container %s not found", id)
	}
	return d, nil
}

func (m *mockDatabaseDockerClient) CreateContainer(ctx context.Context, req docker.CreateContainerRequest) (docker.CreateContainerResult, error) {
	m.createdContainerReq = &req
	id := "cnt-" + req.Name
	labels := map[string]string{}
	for k, v := range req.PlatformLabels {
		labels[k] = v
	}
	for k, v := range req.Labels {
		labels[k] = v
	}
	detail := docker.ContainerDetail{
		ID:     id,
		Name:   req.Name,
		State:  docker.ContainerState{Running: false, Status: "created"},
		Labels: labels,
	}
	m.containers[req.Name] = detail
	m.containers[id] = detail
	return docker.CreateContainerResult{ID: id}, nil
}

func (m *mockDatabaseDockerClient) StartContainer(ctx context.Context, id string) error {
	m.startedContainers = append(m.startedContainers, id)
	if d, ok := m.containers[id]; ok {
		d.State.Running = true
		d.State.Status = "running"
		m.containers[id] = d
		m.containers[d.Name] = d
	}
	return nil
}

func (m *mockDatabaseDockerClient) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.stoppedContainers = append(m.stoppedContainers, id)
	if d, ok := m.containers[id]; ok {
		d.State.Running = false
		d.State.Status = "stopped"
		m.containers[id] = d
		m.containers[d.Name] = d
	}
	return nil
}

func (m *mockDatabaseDockerClient) InspectVolume(ctx context.Context, name string) (docker.VolumeDetail, error) {
	v, ok := m.volumes[name]
	if !ok {
		return docker.VolumeDetail{}, fmt.Errorf("volume %s not found", name)
	}
	return v, nil
}

func (m *mockDatabaseDockerClient) CreateVolume(ctx context.Context, req docker.CreateVolumeRequest) (docker.VolumeSummary, error) {
	m.volumes[req.Name] = docker.VolumeDetail{Name: req.Name, Labels: req.Labels}
	return docker.VolumeSummary{Name: req.Name, Labels: req.Labels}, nil
}

func (m *mockDatabaseDockerClient) InspectNetwork(ctx context.Context, name string) (docker.NetworkDetail, error) {
	n, ok := m.networks[name]
	if !ok {
		return docker.NetworkDetail{}, fmt.Errorf("network %s not found", name)
	}
	return n, nil
}

func (m *mockDatabaseDockerClient) CreateNetwork(ctx context.Context, req docker.CreateNetworkRequest) (string, error) {
	m.networks[req.Name] = docker.NetworkDetail{ID: "net-" + req.Name, Name: req.Name}
	return "net-" + req.Name, nil
}

func (m *mockDatabaseDockerClient) InspectImage(ctx context.Context, ref string) (docker.ImageDetail, error) {
	img, ok := m.images[ref]
	if !ok {
		return docker.ImageDetail{}, fmt.Errorf("image %s not found", ref)
	}
	return img, nil
}

func (m *mockDatabaseDockerClient) PullImage(ctx context.Context, ref string, out io.Writer) error {
	m.images[ref] = docker.ImageDetail{ID: "img-" + ref}
	return nil
}

func TestManager_Provision(t *testing.T) {
	cli := newMockDocker()
	mgr := NewManager(cli, nil)

	req := ProvisionRequest{
		DatabaseID:        "db-uuid-1234",
		Engine:            "postgres",
		EngineVersion:     "16",
		DatabaseName:      "app_prod",
		Username:          "deployuser",
		Password:          "secret_pwd_99",
		StorageVolumeName: "dc-vol-db-app-prod",
		NetworkName:       "dc-net-proj-env",
		CPUMillis:         1000,
		MemoryBytes:       1024 * 1024 * 1024,
		VolumeProtected:   true,
	}

	state, err := mgr.Provision(context.Background(), req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	if state.ContainerName != "dc-db-uuid-1234" {
		t.Errorf("expected container name 'dc-db-uuid-1234', got %s", state.ContainerName)
	}
	if state.Status != "running" {
		t.Errorf("expected status 'running', got %s", state.Status)
	}

	// Verify volume was created with protected label
	vol, ok := cli.volumes["dc-vol-db-app-prod"]
	if !ok {
		t.Fatalf("expected volume 'dc-vol-db-app-prod' to be created")
	}
	if vol.Labels[protocol.LabelProtected] != "true" {
		t.Errorf("expected volume to have deploycore.protected='true', got %s", vol.Labels[protocol.LabelProtected])
	}

	// Verify container configuration
	cReq := cli.createdContainerReq
	if cReq == nil {
		t.Fatalf("expected CreateContainerRequest")
	}

	// Volume mount verify: /var/lib/postgresql/data
	foundMount := false
	for _, m := range cReq.Volumes {
		if m.VolumeName == "dc-vol-db-app-prod" && m.MountPath == "/var/lib/postgresql/data" {
			foundMount = true
		}
	}
	if !foundMount {
		t.Errorf("expected volume mount to /var/lib/postgresql/data")
	}

	// Health check verify — CMD argv, not CMD-SHELL
	if cReq.HealthCheck == nil || len(cReq.HealthCheck.Test) < 4 {
		t.Errorf("expected pg_isready health check argv")
	} else if cReq.HealthCheck.Test[0] != "CMD" || cReq.HealthCheck.Test[1] != "pg_isready" {
		t.Errorf("expected CMD pg_isready health check, got %v", cReq.HealthCheck.Test)
	}

	// Protected label verify
	if cReq.PlatformLabels[protocol.LabelProtected] != "true" {
		t.Errorf("expected container to have deploycore.protected='true'")
	}
}

func TestManager_StartStop(t *testing.T) {
	cli := newMockDocker()
	mgr := NewManager(cli, nil)

	containerName := "dc-db-123"
	cli.containers[containerName] = docker.ContainerDetail{
		ID:   "c-123",
		Name: containerName,
		State: docker.ContainerState{
			Running: true,
			Status:  "running",
		},
		Labels: map[string]string{
			protocol.LabelManaged:     "true",
			"deploycore.service_type": "database",
			"deploycore.database_id":  "123",
		},
	}
	cli.containers["c-123"] = cli.containers[containerName]

	// Stop
	if err := mgr.Stop(context.Background(), "123", 5*time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if len(cli.stoppedContainers) != 1 || cli.stoppedContainers[0] != "c-123" {
		t.Errorf("expected c-123 to be stopped, got: %v", cli.stoppedContainers)
	}

	// Start
	if err := mgr.Start(context.Background(), "123"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if len(cli.startedContainers) != 1 || cli.startedContainers[0] != "c-123" {
		t.Errorf("expected c-123 to be started, got: %v", cli.startedContainers)
	}
}
