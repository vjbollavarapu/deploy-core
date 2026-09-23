package network

import (
	"context"
	"errors"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockDockerClient struct {
	networks map[string]docker.NetworkDetail
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		networks: make(map[string]docker.NetworkDetail),
	}
}

func (m *mockDockerClient) ListNetworks(ctx context.Context) ([]docker.NetworkSummary, error) {
	var list []docker.NetworkSummary
	for _, n := range m.networks {
		list = append(list, docker.NetworkSummary{
			ID:     n.ID,
			Name:   n.Name,
			Driver: n.Driver,
			Scope:  n.Scope,
			Labels: n.Labels,
		})
	}
	return list, nil
}

func (m *mockDockerClient) InspectNetwork(ctx context.Context, idOrName string) (docker.NetworkDetail, error) {
	for id, n := range m.networks {
		if id == idOrName || n.Name == idOrName {
			return n, nil
		}
	}
	return docker.NetworkDetail{}, &docker.AgentError{Code: docker.ErrCodeNotFound, Message: "network not found"}
}

func (m *mockDockerClient) CreateNetwork(ctx context.Context, req docker.CreateNetworkRequest) (string, error) {
	id := "net-" + req.Name
	m.networks[id] = docker.NetworkDetail{
		ID:         id,
		Name:       req.Name,
		Driver:     req.Driver,
		Internal:   req.Internal,
		Labels:     req.Labels,
		Containers: make(map[string]docker.NetworkEndpoint),
	}
	return id, nil
}

func (m *mockDockerClient) RemoveNetwork(ctx context.Context, id string) error {
	for k, n := range m.networks {
		if k == id || n.ID == id {
			delete(m.networks, k)
			return nil
		}
	}
	return &docker.AgentError{Code: docker.ErrCodeNotFound, Message: "network not found"}
}

func TestEnsurePrivateNetwork_CreateAndIdempotency(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	meta := Metadata{
		OrganizationID:  "org-1",
		ProjectSlug:     "daya",
		EnvironmentSlug: "prod",
	}

	// 1. First run: network does not exist -> creates it
	detail, err := mgr.EnsurePrivateNetwork(context.Background(), meta)
	if err != nil {
		t.Fatalf("unexpected error creating private network: %v", err)
	}
	expectedName := "dc-daya-prod-private"
	if detail.Name != expectedName {
		t.Errorf("expected network name %q, got %q", expectedName, detail.Name)
	}
	if detail.Labels[protocol.LabelManaged] != "true" {
		t.Errorf("expected deploycore.managed='true', got %q", detail.Labels[protocol.LabelManaged])
	}

	// 2. Second run: network already exists -> returns existing without creating new
	detail2, err := mgr.EnsurePrivateNetwork(context.Background(), meta)
	if err != nil {
		t.Fatalf("unexpected error on idempotent call: %v", err)
	}
	if detail2.ID != detail.ID {
		t.Errorf("expected same network ID %q, got %q", detail.ID, detail2.ID)
	}
}

func TestEnsurePrivateNetwork_UnmanagedConflict(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	// An unmanaged container squatting on the platform name
	name := "dc-daya-prod-private"
	mock.networks["unmanaged-id"] = docker.NetworkDetail{
		ID:     "unmanaged-id",
		Name:   name,
		Labels: map[string]string{}, // unmanaged!
	}

	meta := Metadata{
		OrganizationID:  "org-1",
		ProjectSlug:     "daya",
		EnvironmentSlug: "prod",
	}

	_, err := mgr.EnsurePrivateNetwork(context.Background(), meta)
	if err == nil {
		t.Fatalf("expected error due to unmanaged conflict, got nil")
	}
	if !errors.Is(err, ErrUnmanagedNetworkConflict) {
		t.Errorf("expected ErrUnmanagedNetworkConflict, got %v", err)
	}
}

func TestEnsureProxyNetwork_Idempotency(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	detail, err := mgr.EnsureProxyNetwork(context.Background())
	if err != nil {
		t.Fatalf("unexpected error ensuring proxy network: %v", err)
	}
	if detail.Name != protocol.ProxyNetworkName {
		t.Errorf("expected proxy network name %q, got %q", protocol.ProxyNetworkName, detail.Name)
	}

	// Ensure second call returns same network
	detail2, err := mgr.EnsureProxyNetwork(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if detail2.ID != detail.ID {
		t.Errorf("expected same network ID %q, got %q", detail.ID, detail2.ID)
	}
}

func TestDeleteNetwork_AccidentalDeletionProtection(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	meta := Metadata{
		OrganizationID:  "org-1",
		ProjectSlug:     "daya",
		EnvironmentSlug: "prod",
	}

	detail, err := mgr.EnsurePrivateNetwork(context.Background(), meta)
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}

	// Attach an active container to the network
	mockNet := mock.networks[detail.ID]
	mockNet.Containers = map[string]docker.NetworkEndpoint{
		"cid-1": {Name: "dc-daya-prod-r49-1", EndpointID: "ep-1"},
	}
	mock.networks[detail.ID] = mockNet

	// Attempt deletion — MUST FAIL to protect connected containers
	err = mgr.DeleteNetwork(context.Background(), detail.ID, "org-1")
	if err == nil {
		t.Fatalf("expected deletion to fail due to connected containers, got nil")
	}
	if !errors.Is(err, ErrConnectedContainersExist) {
		t.Errorf("expected ErrConnectedContainersExist, got %v", err)
	}

	// Detach container
	mockNet.Containers = make(map[string]docker.NetworkEndpoint)
	mock.networks[detail.ID] = mockNet

	// Now deletion should succeed
	err = mgr.DeleteNetwork(context.Background(), detail.ID, "org-1")
	if err != nil {
		t.Fatalf("expected successful deletion of empty network, got %v", err)
	}
}

func TestDeleteNetwork_RefuseUnmanagedNetwork(t *testing.T) {
	mock := newMockDockerClient()
	mgr := NewManager(mock)

	mock.networks["unmanaged-id"] = docker.NetworkDetail{
		ID:     "unmanaged-id",
		Name:   "my-foreign-bridge",
		Labels: map[string]string{}, // unmanaged
	}

	err := mgr.DeleteNetwork(context.Background(), "unmanaged-id", "org-1")
	if err == nil {
		t.Fatalf("expected deletion of unmanaged network to be refused, got nil")
	}
	if !errors.Is(err, ErrUnmanagedNetworkCannotDelete) {
		t.Errorf("expected ErrUnmanagedNetworkCannotDelete, got %v", err)
	}
}
