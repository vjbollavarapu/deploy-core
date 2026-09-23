package network

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrConnectedContainersExist is returned when attempting to remove a network that still has containers attached.
	ErrConnectedContainersExist = errors.New("cannot delete network: active containers are connected")

	// ErrUnmanagedNetworkConflict is returned when an unmanaged network already squats on the desired platform name.
	ErrUnmanagedNetworkConflict = errors.New("cannot create platform network: an unmanaged network already exists with this name")

	// ErrUnmanagedNetworkCannotDelete is returned when trying to delete an unmanaged network without explicit workflow.
	ErrUnmanagedNetworkCannotDelete = errors.New("cannot delete unmanaged network without explicit import/ownership workflow")

	// ErrTenantMismatch indicates the network belongs to a different organization.
	ErrTenantMismatch = errors.New("network belongs to a different organization")
)

// DockerClient represents the subset of Docker operations needed by the network manager.
type DockerClient interface {
	ListNetworks(ctx context.Context) ([]docker.NetworkSummary, error)
	InspectNetwork(ctx context.Context, idOrName string) (docker.NetworkDetail, error)
	CreateNetwork(ctx context.Context, req docker.CreateNetworkRequest) (string, error)
	RemoveNetwork(ctx context.Context, id string) error
}

// Manager handles platform-managed Docker networks lifecycle and safety invariants.
type Manager struct {
	client DockerClient
}

// NewManager creates a new platform network manager.
func NewManager(client DockerClient) *Manager {
	return &Manager{client: client}
}

// EnsurePrivateNetwork idempotently creates or verifies a project private application network.
// Standard naming: dc-<project>-<environment>-private
// Inspects before creating; returns existing network if already verified.
func (m *Manager) EnsurePrivateNetwork(ctx context.Context, meta Metadata) (docker.NetworkDetail, error) {
	if meta.ProjectSlug == "" && meta.ProjectID == "" {
		return docker.NetworkDetail{}, fmt.Errorf("project identifier required")
	}
	if meta.EnvironmentSlug == "" && meta.EnvironmentID == "" {
		return docker.NetworkDetail{}, fmt.Errorf("environment identifier required")
	}

	proj := meta.ProjectSlug
	if proj == "" {
		proj = meta.ProjectID
	}
	env := meta.EnvironmentSlug
	if env == "" {
		env = meta.EnvironmentID
	}

	name, err := FormatPrivateNetworkName(proj, env)
	if err != nil {
		return docker.NetworkDetail{}, err
	}

	meta.NetworkType = protocol.NetworkTypePrivate
	return m.ensureNetwork(ctx, name, meta, false)
}

// EnsureProxyNetwork idempotently creates or verifies the shared deploycore-proxy network.
func (m *Manager) EnsureProxyNetwork(ctx context.Context) (docker.NetworkDetail, error) {
	meta := Metadata{
		NetworkType: protocol.NetworkTypeProxy,
	}
	return m.ensureNetwork(ctx, protocol.ProxyNetworkName, meta, false)
}

// EnsureNetwork creates or verifies any platform network by name and metadata.
func (m *Manager) EnsureNetwork(ctx context.Context, name string, meta Metadata, internal bool) (docker.NetworkDetail, error) {
	return m.ensureNetwork(ctx, name, meta, internal)
}

func (m *Manager) ensureNetwork(ctx context.Context, name string, meta Metadata, internal bool) (docker.NetworkDetail, error) {
	// 1. Inspect before create
	existing, err := m.client.InspectNetwork(ctx, name)
	if err == nil {
		// Network already exists on host — verify ownership
		status := VerifyOwnership(name, existing.Labels, meta.OrganizationID)
		switch status {
		case OwnershipValid:
			// Idempotent success: network already exists with valid platform labels
			return existing, nil
		case OwnershipTenantMismatch:
			return docker.NetworkDetail{}, fmt.Errorf("%w: network %q belongs to another tenant", ErrTenantMismatch, name)
		case OwnershipUntrustedCollision, OwnershipUnmanaged:
			return docker.NetworkDetail{}, fmt.Errorf("%w: %q (id %s)", ErrUnmanagedNetworkConflict, name, existing.ID)
		}
	}

	// If error is not a NotFound error, return it
	if !isNotFoundError(err) {
		return docker.NetworkDetail{}, fmt.Errorf("failed to inspect network before create: %w", err)
	}

	// 2. Network does not exist; create it with platform labels
	req := docker.CreateNetworkRequest{
		Name:     name,
		Driver:   "bridge",
		Internal: internal,
		Labels:   meta.Labels(),
		Options: map[string]string{
			"com.docker.network.bridge.enable_icc":           "true",
			"com.docker.network.bridge.name":                 truncateInterfaceName(name),
			"com.docker.network.bridge.enable_ip_masquerade": "true",
		},
	}

	id, err := m.client.CreateNetwork(ctx, req)
	if err != nil {
		return docker.NetworkDetail{}, fmt.Errorf("failed to create network %q: %w", name, err)
	}

	// 3. Inspect newly created network to return full details
	detail, err := m.client.InspectNetwork(ctx, id)
	if err != nil {
		// Creation succeeded even if inspect failed
		return docker.NetworkDetail{ID: id, Name: name, Labels: req.Labels}, nil
	}
	return detail, nil
}

// DeleteNetwork safely removes a network:
// 1. Inspects network.
// 2. Verifies platform ownership (refuses unmanaged networks).
// 3. Verifies tenant isolation if expectedOrgID is provided.
// 4. PREVENTS accidental deletion if connected resources exist (len(Containers) > 0).
func (m *Manager) DeleteNetwork(ctx context.Context, idOrName string, expectedOrgID string) error {
	detail, err := m.client.InspectNetwork(ctx, idOrName)
	if err != nil {
		if isNotFoundError(err) {
			return nil // idempotent delete
		}
		return err
	}

	// Verify ownership
	status := VerifyOwnership(detail.Name, detail.Labels, expectedOrgID)
	switch status {
	case OwnershipTenantMismatch:
		return fmt.Errorf("%w: network %q belongs to another tenant", ErrTenantMismatch, detail.Name)
	case OwnershipUntrustedCollision, OwnershipUnmanaged:
		return fmt.Errorf("%w: %q is not owned by DeployCore", ErrUnmanagedNetworkCannotDelete, detail.Name)
	}

	// Safeguard: Prevent accidental deletion when connected resources exist
	if len(detail.Containers) > 0 {
		var containerNames []string
		for cid, ep := range detail.Containers {
			cName := ep.Name
			if cName == "" {
				cName = cid[:min(12, len(cid))]
			}
			containerNames = append(containerNames, cName)
		}
		return fmt.Errorf("%w (%d active containers: %s)", ErrConnectedContainersExist, len(detail.Containers), strings.Join(containerNames, ", "))
	}

	// Remove network safely
	return m.client.RemoveNetwork(ctx, detail.ID)
}

// InspectNetwork inspects a network and verifies platform ownership.
func (m *Manager) InspectNetwork(ctx context.Context, idOrName string) (docker.NetworkDetail, OwnershipStatus, error) {
	detail, err := m.client.InspectNetwork(ctx, idOrName)
	if err != nil {
		return docker.NetworkDetail{}, OwnershipUnmanaged, err
	}
	status := VerifyOwnership(detail.Name, detail.Labels, "")
	return detail, status, nil
}

// isNotFoundError checks if an error represents Docker resource not found.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var ae *docker.AgentError
	if errors.As(err, &ae) {
		return ae.Code == docker.ErrCodeNotFound
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such network") || strings.Contains(msg, "404")
}

// truncateInterfaceName creates an interface name <= 15 chars (Linux IFNAMSIZ limit)
func truncateInterfaceName(name string) string {
	clean := strings.ReplaceAll(name, "-", "")
	if len(clean) > 15 {
		return clean[:15]
	}
	return clean
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
