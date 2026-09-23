package volume

import (
	"context"
	"errors"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockDockerClient struct {
	volumes    map[string]docker.VolumeDetail
	containers map[string]docker.ContainerDetail
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		volumes:    make(map[string]docker.VolumeDetail),
		containers: make(map[string]docker.ContainerDetail),
	}
}

func (m *mockDockerClient) ListVolumes(ctx context.Context) ([]docker.VolumeSummary, error) {
	var list []docker.VolumeSummary
	for _, v := range m.volumes {
		list = append(list, docker.VolumeSummary{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			Labels:     v.Labels,
			Scope:      v.Scope,
		})
	}
	return list, nil
}

func (m *mockDockerClient) InspectVolume(ctx context.Context, name string) (docker.VolumeDetail, error) {
	v, ok := m.volumes[name]
	if !ok {
		return docker.VolumeDetail{}, &docker.AgentError{Code: docker.ErrCodeNotFound, Message: "volume not found"}
	}
	return v, nil
}

func (m *mockDockerClient) CreateVolume(ctx context.Context, req docker.CreateVolumeRequest) (docker.VolumeSummary, error) {
	m.volumes[req.Name] = docker.VolumeDetail{
		Name:       req.Name,
		Driver:     req.Driver,
		Mountpoint: "/var/lib/docker/volumes/" + req.Name + "/_data",
		Labels:     req.Labels,
		Usage:      &docker.VolumeUsage{SizeBytes: 1048576, RefCount: 0},
	}
	return docker.VolumeSummary{
		Name:       req.Name,
		Driver:     req.Driver,
		Mountpoint: "/var/lib/docker/volumes/" + req.Name + "/_data",
		Labels:     req.Labels,
	}, nil
}

func (m *mockDockerClient) RemoveVolume(ctx context.Context, name string, force bool) error {
	if _, ok := m.volumes[name]; !ok {
		return &docker.AgentError{Code: docker.ErrCodeNotFound, Message: "volume not found"}
	}
	delete(m.volumes, name)
	return nil
}

func (m *mockDockerClient) ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error) {
	var list []docker.ContainerSummary
	for _, c := range m.containers {
		list = append(list, docker.ContainerSummary{
			ID:     c.ID,
			Names:  []string{c.Name},
			Labels: c.Labels,
		})
	}
	return list, nil
}

func (m *mockDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	c, ok := m.containers[id]
	if !ok {
		return docker.ContainerDetail{}, &docker.AgentError{Code: docker.ErrCodeNotFound, Message: "container not found"}
	}
	return c, nil
}

func TestEnsureVolume_CreateAndIdempotency(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	meta := Metadata{
		OrganizationID: "org-1",
		VolumeID:       "vol-1",
	}

	// 1. Create new volume
	detail, err := mgr.EnsureVolume(context.Background(), "dc-app-data", meta, "local", nil)
	if err != nil {
		t.Fatalf("unexpected error ensuring volume: %v", err)
	}
	if detail.Name != "dc-app-data" {
		t.Errorf("expected dc-app-data, got %q", detail.Name)
	}
	if detail.Labels[protocol.LabelManaged] != "true" {
		t.Errorf("expected deploycore.managed='true', got %q", detail.Labels[protocol.LabelManaged])
	}

	// 2. Idempotent call
	detail2, err := mgr.EnsureVolume(context.Background(), "dc-app-data", meta, "local", nil)
	if err != nil {
		t.Fatalf("unexpected error on idempotent call: %v", err)
	}
	if detail2.Name != detail.Name {
		t.Errorf("expected matching volume detail on idempotent call")
	}
}

func TestEnsureVolume_UnmanagedConflict(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	// Pre-populate unmanaged volume
	mock.volumes["dc-app-data"] = docker.VolumeDetail{
		Name:   "dc-app-data",
		Labels: map[string]string{}, // unmanaged!
	}

	meta := Metadata{
		OrganizationID: "org-1",
		VolumeID:       "vol-1",
	}

	_, err := mgr.EnsureVolume(context.Background(), "dc-app-data", meta, "local", nil)
	if err == nil {
		t.Fatalf("expected conflict error for unmanaged volume, got nil")
	}
	if !errors.Is(err, ErrUnmanagedVolumeConflict) {
		t.Errorf("expected ErrUnmanagedVolumeConflict, got %v", err)
	}
}

func TestDeleteVolume_BlockIfAttached(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	meta := Metadata{
		OrganizationID: "org-1",
		VolumeID:       "vol-1",
	}
	vol, err := mgr.EnsureVolume(context.Background(), "dc-app-data", meta, "local", nil)
	if err != nil {
		t.Fatalf("failed to create volume: %v", err)
	}

	// Attach container
	mock.containers["c-1"] = docker.ContainerDetail{
		ID:   "c-1",
		Name: "/dc-app-r49-1",
		Mounts: []docker.MountPoint{
			{Type: "volume", Source: vol.Name, Destination: "/data", RW: true},
		},
		State: docker.ContainerState{Running: true},
	}

	// 1. Delete without force MUST BE BLOCKED
	err = mgr.DeleteVolume(context.Background(), vol.Name, false, "org-1")
	if err == nil {
		t.Fatalf("expected delete to fail for attached volume, got nil")
	}
	if !errors.Is(err, ErrVolumeAttached) {
		t.Errorf("expected ErrVolumeAttached, got %v", err)
	}

	// 2. Delete with force authorized by CP policy MUST SUCCEED
	err = mgr.DeleteVolume(context.Background(), vol.Name, true, "org-1")
	if err != nil {
		t.Fatalf("expected forced delete to succeed, got %v", err)
	}

	// Confirm volume was deleted
	if _, ok := mock.volumes[vol.Name]; ok {
		t.Errorf("volume should have been deleted from storage")
	}
}

func TestDeleteVolume_RefuseUnmanaged(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	mock.volumes["foreign-vol"] = docker.VolumeDetail{
		Name:   "foreign-vol",
		Labels: map[string]string{},
	}

	err := mgr.DeleteVolume(context.Background(), "foreign-vol", false, "org-1")
	if err == nil {
		t.Fatalf("expected error deleting unmanaged volume, got nil")
	}
	if !errors.Is(err, ErrUnmanagedVolumeCannotDelete) {
		t.Errorf("expected ErrUnmanagedVolumeCannotDelete, got %v", err)
	}
}

func TestInspectVolume_WithAttachmentsAndUsage(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	meta := Metadata{
		OrganizationID: "org-1",
		VolumeID:       "vol-1",
		ApplicationID:  "app-1",
	}
	vol, err := mgr.EnsureVolume(context.Background(), "dc-app-data", meta, "local", nil)
	if err != nil {
		t.Fatalf("failed to create volume: %v", err)
	}

	mock.containers["c-1"] = docker.ContainerDetail{
		ID:   "c-1",
		Name: "/dc-app-r49-1",
		Mounts: []docker.MountPoint{
			{Type: "volume", Source: vol.Name, Destination: "/data", RW: true},
		},
		State: docker.ContainerState{Running: true},
	}

	inspection, err := mgr.InspectVolume(context.Background(), vol.Name)
	if err != nil {
		t.Fatalf("failed to inspect volume: %v", err)
	}

	if inspection.Ownership != OwnershipValid {
		t.Errorf("expected OwnershipValid, got %s", inspection.Ownership)
	}
	if len(inspection.AttachedContainers) != 1 {
		t.Fatalf("expected 1 attached container, got %d", len(inspection.AttachedContainers))
	}
	if inspection.AttachedContainers[0].ID != "c-1" {
		t.Errorf("expected container c-1, got %s", inspection.AttachedContainers[0].ID)
	}
	if inspection.SizeBytes <= 0 {
		t.Errorf("expected positive volume size, got %d", inspection.SizeBytes)
	}
}
