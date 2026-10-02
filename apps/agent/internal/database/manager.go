package database

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// DockerClient represents the Docker engine subset required for stateful database execution.
type DockerClient interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	CreateContainer(ctx context.Context, req docker.CreateContainerRequest) (docker.CreateContainerResult, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string, timeout time.Duration) error
	InspectVolume(ctx context.Context, name string) (docker.VolumeDetail, error)
	CreateVolume(ctx context.Context, req docker.CreateVolumeRequest) (docker.VolumeSummary, error)
	InspectNetwork(ctx context.Context, name string) (docker.NetworkDetail, error)
	CreateNetwork(ctx context.Context, req docker.CreateNetworkRequest) (string, error)
	RemoveContainer(ctx context.Context, id string, force bool) error
	InspectImage(ctx context.Context, ref string) (docker.ImageDetail, error)
	PullImage(ctx context.Context, ref string, out io.Writer) error
}

// Manager orchestrates lifecycle management of stateful database containers.
type Manager struct {
	cli DockerClient
	log *slog.Logger
}

// NewManager constructs a database Manager.
func NewManager(cli DockerClient, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{cli: cli, log: log}
}

// FormatContainerName returns the stable platform container name for a database.
func FormatContainerName(databaseID string) string {
	cleanID := strings.TrimPrefix(strings.TrimSpace(databaseID), "db-")
	return fmt.Sprintf("dc-db-%s", cleanID)
}

// FormatImageName maps engine and engineVersion into a valid Docker image reference.
// Call ValidateImageVersion before provision; this helper assumes a validated version.
func FormatImageName(engine, engineVersion string) string {
	_ = engine
	ver := strings.TrimSpace(engineVersion)
	if ver == "" {
		ver = "16"
	}
	return fmt.Sprintf("postgres:%s-alpine", ver)
}

// ValidateImageVersion rejects opaque/non-numeric engine versions before provision.
func ValidateImageVersion(engineVersion string) error {
	ver := strings.TrimSpace(engineVersion)
	if ver == "" {
		return nil
	}
	for _, r := range ver {
		if (r < '0' || r > '9') && r != '.' {
			return fmt.Errorf("engineVersion must be a numeric PostgreSQL version, got %q", engineVersion)
		}
	}
	if strings.HasPrefix(ver, ".") || strings.HasSuffix(ver, ".") || strings.Contains(ver, "..") {
		return fmt.Errorf("engineVersion must be a numeric PostgreSQL version, got %q", engineVersion)
	}
	return nil
}

// Provision ensures the persistent volume, network, pulls the image, creates the PostgreSQL container,
// and starts it. Invariant: database data is placed on a persistent protected volume, never a disposable container filesystem.
func (m *Manager) Provision(ctx context.Context, req ProvisionRequest) (*DatabaseState, error) {
	if strings.TrimSpace(req.DatabaseID) == "" {
		return nil, errors.New("databaseId cannot be empty")
	}
	if strings.TrimSpace(req.StorageVolumeName) == "" {
		return nil, errors.New("storageVolumeName cannot be empty")
	}
	if strings.TrimSpace(req.DatabaseName) == "" {
		return nil, errors.New("databaseName cannot be empty")
	}
	if strings.TrimSpace(req.Username) == "" {
		return nil, errors.New("username cannot be empty")
	}
	if strings.TrimSpace(req.Password) == "" {
		return nil, errors.New("password cannot be empty")
	}
	if err := ValidateImageVersion(req.EngineVersion); err != nil {
		return nil, err
	}
	if err := validatePrivateAttachment(req.NetworkName, req.DNSAlias); err != nil {
		return nil, err
	}

	containerName := FormatContainerName(req.DatabaseID)
	imageName := FormatImageName(req.Engine, req.EngineVersion)

	m.log.Info("provisioning stateful database",
		slog.String("database_id", req.DatabaseID),
		slog.String("container_name", containerName),
		slog.String("image", imageName),
		slog.String("volume", req.StorageVolumeName),
	)

	// 1. Ensure persistent volume exists
	if vol, err := m.cli.InspectVolume(ctx, req.StorageVolumeName); err == nil {
		if err := VerifyManagedDatabaseLabels(vol.Labels, req.DatabaseID); err != nil {
			return nil, fmt.Errorf("refusing to reuse volume %s: %w", req.StorageVolumeName, err)
		}
	} else {
		m.log.Info("creating persistent database volume", slog.String("volume", req.StorageVolumeName))
		volReq := docker.CreateVolumeRequest{
			Name: req.StorageVolumeName,
			Labels: map[string]string{
				protocol.LabelManaged:     "true",
				protocol.LabelProtected:   "true",
				"deploycore.service_type": "database",
				"deploycore.database_id":  req.DatabaseID,
			},
		}
		if _, err := m.cli.CreateVolume(ctx, volReq); err != nil {
			return nil, fmt.Errorf("failed to create persistent database volume %s: %w", req.StorageVolumeName, err)
		}
	}

	// 2. Ensure image exists locally or pull it
	if _, err := m.cli.InspectImage(ctx, imageName); err != nil {
		m.log.Info("pulling database image", slog.String("image", imageName))
		if pullErr := m.cli.PullImage(ctx, imageName, nil); pullErr != nil {
			return nil, fmt.Errorf("failed to pull database image %s: %w", imageName, pullErr)
		}
	}

	// 3. An existing container must already be on this private network with the alias.
	// Otherwise recreate it onto that network and keep the named volume.
	existing, err := m.cli.InspectContainer(ctx, containerName)
	if err == nil {
		if ownErr := VerifyManagedDatabaseContainer(existing, req.DatabaseID); ownErr != nil {
			return nil, fmt.Errorf("refusing to reuse container %s: %w", containerName, ownErr)
		}
		if privateAttachmentReady(existing, req.NetworkName, req.DNSAlias) {
			if !existing.State.Running {
				if startErr := m.cli.StartContainer(ctx, existing.ID); startErr != nil {
					return nil, fmt.Errorf("failed to start existing database container: %w", startErr)
				}
			}
			return &DatabaseState{
				DatabaseID:    req.DatabaseID,
				ContainerID:   existing.ID,
				ContainerName: containerName,
				Status:        "running",
				UpdatedAt:     time.Now().UTC(),
			}, nil
		}
		if existing.State.Running {
			if stopErr := m.cli.StopContainer(ctx, existing.ID, 15*time.Second); stopErr != nil {
				return nil, fmt.Errorf("failed to stop database container for private-network attach: %w", stopErr)
			}
		}
		if rmErr := m.cli.RemoveContainer(ctx, existing.ID, true); rmErr != nil {
			return nil, fmt.Errorf("failed to recreate database container onto private network: %w", rmErr)
		}
	}

	// 4. Create container on the private network only. No host port and no proxy network.
	createReq := docker.CreateContainerRequest{
		Name:  containerName,
		Image: imageName,
		Env: []string{
			"POSTGRES_DB=" + req.DatabaseName,
			"POSTGRES_USER=" + req.Username,
			"POSTGRES_PASSWORD=" + req.Password,
			"PGDATA=/var/lib/postgresql/data/pgdata",
		},
		Volumes: []docker.VolumeMount{
			{
				VolumeName: req.StorageVolumeName,
				MountPath:  "/var/lib/postgresql/data",
				ReadOnly:   false,
			},
		},
		Networks: []string{req.NetworkName},
		NetworkAliases: map[string][]string{
			req.NetworkName: {req.DNSAlias},
		},
		CPUMillis:     req.CPUMillis,
		MemoryBytes:   req.MemoryBytes,
		RestartPolicy: docker.RestartUnlessStopped,
		HealthCheck: &docker.HealthCheckConfig{
			// CMD argv — never CMD-SHELL with interpolated user/db names.
			Test:        []string{"CMD", "pg_isready", "-U", req.Username, "-d", req.DatabaseName},
			Interval:    5 * time.Second,
			Timeout:     3 * time.Second,
			Retries:     5,
			StartPeriod: 10 * time.Second,
		},
		PlatformLabels: map[string]string{
			protocol.LabelManaged:     "true",
			protocol.LabelProtected:   "true",
			"deploycore.service_type": "database",
			"deploycore.database_id":  req.DatabaseID,
		},
		// Empty policy: CreateContainer drops ALL capabilities and adds the
		// shared baseline. There is no database-specific capability list.
		Policy: &docker.PrivilegedPolicy{},
	}

	res, err := m.cli.CreateContainer(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create database container: %w", err)
	}

	// 5. Start container
	if err := m.cli.StartContainer(ctx, res.ID); err != nil {
		return nil, fmt.Errorf("failed to start database container: %w", err)
	}

	return &DatabaseState{
		DatabaseID:    req.DatabaseID,
		ContainerID:   res.ID,
		ContainerName: containerName,
		Status:        "running",
		UpdatedAt:     time.Now().UTC(),
	}, nil
}

// Start starts a stopped database container.
func (m *Manager) Start(ctx context.Context, databaseID string) error {
	containerName := FormatContainerName(databaseID)
	detail, err := m.cli.InspectContainer(ctx, containerName)
	if err != nil {
		return fmt.Errorf("database container %s not found: %w", containerName, err)
	}
	if err := VerifyManagedDatabaseContainer(detail, databaseID); err != nil {
		return err
	}
	if detail.State.Running {
		return nil // idempotent
	}
	return m.cli.StartContainer(ctx, detail.ID)
}

// Stop stops a running database container gracefully.
func (m *Manager) Stop(ctx context.Context, databaseID string, timeout time.Duration) error {
	containerName := FormatContainerName(databaseID)
	detail, err := m.cli.InspectContainer(ctx, containerName)
	if err != nil {
		return fmt.Errorf("database container %s not found: %w", containerName, err)
	}
	if err := VerifyManagedDatabaseContainer(detail, databaseID); err != nil {
		return err
	}
	if !detail.State.Running {
		return nil // idempotent
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return m.cli.StopContainer(ctx, detail.ID, timeout)
}

func validatePrivateAttachment(networkName, alias string) error {
	networkName = strings.TrimSpace(networkName)
	alias = strings.TrimSpace(alias)
	switch networkName {
	case "", "bridge", "default", "host", "none", protocol.ProxyNetworkName:
		return fmt.Errorf("database requires the project private network, got %q", networkName)
	}
	if !protocol.ValidDNSAlias(alias) {
		return fmt.Errorf("database DNS alias %q is not valid", alias)
	}
	return nil
}

func privateAttachmentReady(detail docker.ContainerDetail, networkName, alias string) bool {
	if detail.NetworkMode != networkName {
		return false
	}
	if attachedTo(detail, protocol.ProxyNetworkName) {
		return false
	}
	names := attachmentNames(detail)
	if len(names) != 1 || !names[networkName] {
		return false
	}
	for _, candidate := range detail.Aliases[networkName] {
		if candidate == alias {
			return true
		}
	}
	return false
}

func attachedTo(detail docker.ContainerDetail, name string) bool {
	if _, ok := detail.Networks[name]; ok {
		return true
	}
	_, ok := detail.Aliases[name]
	return ok
}

func attachmentNames(detail docker.ContainerDetail) map[string]bool {
	names := map[string]bool{}
	for name := range detail.Networks {
		if strings.TrimSpace(name) != "" {
			names[name] = true
		}
	}
	for name := range detail.Aliases {
		if strings.TrimSpace(name) != "" {
			names[name] = true
		}
	}
	return names
}
