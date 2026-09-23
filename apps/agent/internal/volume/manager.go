package volume

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrVolumeAttached is returned when trying to delete a volume that is mounted by containers without force authorization.
	ErrVolumeAttached = errors.New("cannot delete volume: volume is currently attached to one or more containers")

	// ErrUnmanagedVolumeConflict is returned when an unmanaged volume already squats on the desired platform name.
	ErrUnmanagedVolumeConflict = errors.New("cannot create platform volume: an unmanaged volume already exists with this name")

	// ErrUnmanagedVolumeCannotDelete is returned when attempting to delete an unmanaged volume.
	ErrUnmanagedVolumeCannotDelete = errors.New("cannot delete unmanaged volume without explicit import/ownership workflow")

	// ErrTenantMismatch indicates the volume belongs to a different organization.
	ErrTenantMismatch = errors.New("volume belongs to a different organization")
)

// DockerClient represents the Docker daemon methods required for volume lifecycle and attachment checking.
type DockerClient interface {
	ListVolumes(ctx context.Context) ([]docker.VolumeSummary, error)
	InspectVolume(ctx context.Context, name string) (docker.VolumeDetail, error)
	CreateVolume(ctx context.Context, req docker.CreateVolumeRequest) (docker.VolumeSummary, error)
	RemoveVolume(ctx context.Context, name string, force bool) error
	ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error)
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
}

// AttachedContainer describes a container attached to a volume.
type AttachedContainer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
	Running   bool   `json:"running"`
}

// VolumeInspection contains full inspected details, ownership proof, attachments, and disk usage.
type VolumeInspection struct {
	Detail             docker.VolumeDetail `json:"detail"`
	Metadata           Metadata            `json:"metadata"`
	Ownership          OwnershipStatus     `json:"ownership"`
	AttachedContainers []AttachedContainer `json:"attachedContainers"`
	SizeBytes          int64               `json:"sizeBytes"`
}

// Manager orchestrates managed volumes ensuring platform ownership and data loss safeguards.
type Manager struct {
	client DockerClient
}

// NewManager constructs a platform volume manager.
func NewManager(client DockerClient) *Manager {
	return &Manager{client: client}
}

// EnsureVolume creates or idempotently returns a platform-managed volume.
// Inspects before create to avoid accidental overwrites or duplicate errors.
func (m *Manager) EnsureVolume(ctx context.Context, name string, meta Metadata, driver string, driverOpts map[string]string) (docker.VolumeDetail, error) {
	if err := ValidateVolumeName(name); err != nil {
		return docker.VolumeDetail{}, err
	}
	if err := meta.Validate(); err != nil {
		return docker.VolumeDetail{}, err
	}

	meta.VolumeName = name

	// 1. Inspect before create
	existing, err := m.client.InspectVolume(ctx, name)
	if err == nil {
		// Volume exists on host — verify platform ownership
		status := VerifyOwnership(name, existing.Labels, meta.OrganizationID)
		switch status {
		case OwnershipValid:
			return existing, nil // Idempotent success
		case OwnershipTenantMismatch:
			return docker.VolumeDetail{}, fmt.Errorf("%w: volume %q belongs to another tenant", ErrTenantMismatch, name)
		case OwnershipUntrustedCollision, OwnershipUnmanaged:
			return docker.VolumeDetail{}, fmt.Errorf("%w: %q", ErrUnmanagedVolumeConflict, name)
		}
	}

	if !isNotFoundError(err) {
		return docker.VolumeDetail{}, fmt.Errorf("failed to inspect volume before create: %w", err)
	}

	// 2. Volume does not exist; create it with platform ownership labels
	if driver == "" {
		driver = "local"
	}

	req := docker.CreateVolumeRequest{
		Name:       name,
		Driver:     driver,
		DriverOpts: driverOpts,
		Labels:     meta.Labels(),
	}

	_, err = m.client.CreateVolume(ctx, req)
	if err != nil {
		return docker.VolumeDetail{}, fmt.Errorf("failed to create volume %q: %w", name, err)
	}

	return m.client.InspectVolume(ctx, name)
}

// InspectVolume inspects a volume, resolves its ownership, detects attached containers, and computes usage.
func (m *Manager) InspectVolume(ctx context.Context, name string) (VolumeInspection, error) {
	detail, err := m.client.InspectVolume(ctx, name)
	if err != nil {
		return VolumeInspection{}, err
	}

	ownership := VerifyOwnership(detail.Name, detail.Labels, "")
	meta, _ := ExtractMetadata(detail.Labels)

	// Detect attached containers
	attached, err := m.FindAttachedContainers(ctx, detail.Name)
	if err != nil {
		// Non-fatal, return empty list if listing fails
		attached = nil
	}

	// Calculate size metadata
	var sizeBytes int64
	if detail.Usage != nil && detail.Usage.SizeBytes > 0 {
		sizeBytes = detail.Usage.SizeBytes
	} else if detail.Mountpoint != "" {
		sizeBytes = CalculateMountpointDiskUsage(detail.Mountpoint)
	}

	return VolumeInspection{
		Detail:             detail,
		Metadata:           meta,
		Ownership:          ownership,
		AttachedContainers: attached,
		SizeBytes:          sizeBytes,
	}, nil
}

// DeleteVolume removes a volume only when safely authorized:
// - Explicit destructive delete command required.
// - Blocks deletion if attached to any container unless force is explicitly granted.
// - Refuses deletion of unmanaged volumes.
func (m *Manager) DeleteVolume(ctx context.Context, name string, force bool, expectedOrgID string) error {
	detail, err := m.client.InspectVolume(ctx, name)
	if err != nil {
		if isNotFoundError(err) {
			return nil // Idempotent delete
		}
		return err
	}

	// Verify platform ownership
	status := VerifyOwnership(detail.Name, detail.Labels, expectedOrgID)
	switch status {
	case OwnershipTenantMismatch:
		return fmt.Errorf("%w: volume %q belongs to another tenant", ErrTenantMismatch, detail.Name)
	case OwnershipUntrustedCollision, OwnershipUnmanaged:
		return fmt.Errorf("%w: %q is not owned by DeployCore", ErrUnmanagedVolumeCannotDelete, detail.Name)
	}

	// Check if any containers are currently attached to this volume
	attached, err := m.FindAttachedContainers(ctx, detail.Name)
	if err != nil {
		return fmt.Errorf("failed to check volume attachments: %w", err)
	}

	if len(attached) > 0 {
		if !force {
			var names []string
			for _, a := range attached {
				n := a.Name
				if n == "" {
					n = a.ID[:min(12, len(a.ID))]
				}
				names = append(names, n)
			}
			return fmt.Errorf("%w (%d containers attached: %s; requires authorized force policy)",
				ErrVolumeAttached, len(attached), strings.Join(names, ", "))
		}
	}

	return m.client.RemoveVolume(ctx, detail.Name, force)
}

// FindAttachedContainers scans all containers on the host (running and stopped)
// to identify any container mounting the specified volume.
func (m *Manager) FindAttachedContainers(ctx context.Context, volumeName string) ([]AttachedContainer, error) {
	containers, err := m.client.ListContainers(ctx, true)
	if err != nil {
		return nil, err
	}

	var attached []AttachedContainer
	for _, c := range containers {
		// Inspect container mounts
		detail, err := m.client.InspectContainer(ctx, c.ID)
		if err != nil {
			continue
		}

		for _, m := range detail.Mounts {
			if m.Type == "volume" && m.Source == volumeName {
				cName := ""
				if len(c.Names) > 0 {
					cName = strings.TrimPrefix(c.Names[0], "/")
				}
				attached = append(attached, AttachedContainer{
					ID:        c.ID,
					Name:      cName,
					MountPath: m.Destination,
					ReadOnly:  !m.RW,
					Running:   detail.State.Running,
				})
			}
		}
	}

	return attached, nil
}

// AttachVolume validates that a volume and container can be attached.
func (m *Manager) AttachVolume(ctx context.Context, volumeName string, containerID string, mountPath string, readOnly bool) (map[string]any, error) {
	inspection, err := m.InspectVolume(ctx, volumeName)
	if err != nil {
		return nil, fmt.Errorf("volume %q not found or inaccessible: %w", volumeName, err)
	}
	if inspection.Ownership != OwnershipValid {
		return nil, fmt.Errorf("cannot attach volume %q: unverified ownership (%s)", volumeName, inspection.Ownership)
	}

	cDetail, err := m.client.InspectContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("container %q not found: %w", containerID, err)
	}

	// Verify tenant match if container has platform labels
	cOrg := cDetail.Labels[protocol.LabelOrganizationID]
	if cOrg != "" && inspection.Metadata.OrganizationID != "" && cOrg != inspection.Metadata.OrganizationID {
		return nil, fmt.Errorf("%w: container and volume belong to different tenants", ErrTenantMismatch)
	}

	return map[string]any{
		"volumeName":  volumeName,
		"containerId": containerID,
		"mountPath":   mountPath,
		"readOnly":    readOnly,
		"status":      "attached",
	}, nil
}

// DetachVolume confirms that a volume detachment can be executed safely.
func (m *Manager) DetachVolume(ctx context.Context, volumeName string, containerID string) (map[string]any, error) {
	if _, err := m.client.InspectVolume(ctx, volumeName); err != nil {
		return nil, fmt.Errorf("volume %q not found: %w", volumeName, err)
	}

	cDetail, err := m.client.InspectContainer(ctx, containerID)
	if err == nil && cDetail.State.Running {
		// Warning/informational: Detaching live storage requires container stop/restart
		return map[string]any{
			"volumeName":  volumeName,
			"containerId": containerID,
			"warning":     "container is currently running; detachment takes effect after restart",
			"detached":    true,
		}, nil
	}

	return map[string]any{
		"volumeName":  volumeName,
		"containerId": containerID,
		"detached":    true,
	}, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var ae *docker.AgentError
	if errors.As(err, &ae) {
		return ae.Code == docker.ErrCodeNotFound
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such volume") || strings.Contains(msg, "404")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
