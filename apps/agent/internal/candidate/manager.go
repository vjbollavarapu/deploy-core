package candidate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/appcontainer"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/apps/agent/internal/volume"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrStaleNonCandidate indicates an existing non-candidate container occupies the candidate container name.
	ErrStaleNonCandidate = errors.New("cannot create candidate: active non-candidate container already exists with this name")
	// ErrStartupTimeout indicates the container failed to enter running state within timeout.
	ErrStartupTimeout = errors.New("candidate container failed to start within timeout")
	// ErrPrematureExit indicates the container terminated prematurely during startup.
	ErrPrematureExit = errors.New("candidate container terminated prematurely")
)

// NetworkManager defines the network interface needed by CandidateManager.
type NetworkManager interface {
	EnsureNetwork(ctx context.Context, name string, meta network.Metadata, internal bool) (docker.NetworkDetail, error)
}

// VolumeManager defines the volume interface needed by CandidateManager.
type VolumeManager interface {
	EnsureVolume(ctx context.Context, name string, meta volume.Metadata, driver string, driverOpts map[string]string) (docker.VolumeDetail, error)
}

// Client defines the complete Docker engine methods required by CandidateManager.
type Client interface {
	InspectImage(ctx context.Context, ref string) (docker.ImageDetail, error)
	PullImageWithOptions(ctx context.Context, opts docker.PullImageOptions) (docker.PullImageResult, error)
	InspectNetwork(ctx context.Context, idOrName string) (docker.NetworkDetail, error)
	CreateNetwork(ctx context.Context, req docker.CreateNetworkRequest) (string, error)
	InspectVolume(ctx context.Context, name string) (docker.VolumeDetail, error)
	CreateVolume(ctx context.Context, req docker.CreateVolumeRequest) (docker.VolumeSummary, error)
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	CreateContainer(ctx context.Context, req docker.CreateContainerRequest) (docker.CreateContainerResult, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, id string, force bool) error
	ExecContainer(ctx context.Context, id string, cmd []string) (docker.ExecResult, error)
}

// Manager coordinates the candidate deployment lifecycle.
type Manager struct {
	client     Client
	networkMgr NetworkManager
	volumeMgr  VolumeManager
	log        *slog.Logger
}

// NewManager creates a new candidate deployment manager.
func NewManager(client Client, netMgr NetworkManager, volMgr VolumeManager, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		client:     client,
		networkMgr: netMgr,
		volumeMgr:  volMgr,
		log:        log,
	}
}

// StartCandidate executes the 8-step candidate deployment primitive:
// 1. Ensure image exists.
// 2. Ensure required networks.
// 3. Ensure volumes.
// 4. Create candidate container.
// 5. Start.
// 6. Wait for runtime start.
// 7. Perform health checks.
// 8. Report candidate READY only after policy satisfied.
func (m *Manager) StartCandidate(ctx context.Context, spec CandidateSpec) (CandidateResult, error) {
	startTime := time.Now()

	// Validate metadata & force candidate status
	spec.Metadata.IsCandidate = true
	if err := spec.Metadata.Validate(); err != nil {
		return CandidateResult{}, fmt.Errorf("invalid candidate metadata: %w", err)
	}

	startupTimeout := spec.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = 30 * time.Second
	}

	m.log.Info("starting candidate deployment flow",
		slog.String("applicationId", spec.Metadata.ApplicationID),
		slog.String("revisionId", spec.Metadata.RevisionID),
		slog.Int("instance", spec.Metadata.Instance),
		slog.String("image", spec.Image),
	)

	// Step 1: Ensure image exists
	imgDetail, err := m.ensureImage(ctx, spec)
	if err != nil {
		return CandidateResult{}, fmt.Errorf("step 1 (ensure image) failed: %w", err)
	}

	// Step 2: Ensure required networks
	if err := m.ensureNetworks(ctx, spec); err != nil {
		return CandidateResult{}, fmt.Errorf("step 2 (ensure networks) failed: %w", err)
	}

	// Step 3: Ensure volumes
	if err := m.ensureVolumes(ctx, spec); err != nil {
		return CandidateResult{}, fmt.Errorf("step 3 (ensure volumes) failed: %w", err)
	}

	// Step 4: Create candidate container (with platform labels and proxy traffic isolation)
	containerID, containerName, err := m.createCandidateContainer(ctx, spec)
	if err != nil {
		return CandidateResult{}, fmt.Errorf("step 4 (create candidate container) failed: %w", err)
	}

	// Step 5: Start container
	if err := m.client.StartContainer(ctx, containerID); err != nil {
		return CandidateResult{}, fmt.Errorf("step 5 (start candidate container) failed: %w", err)
	}

	// Step 6: Wait for runtime start
	detail, err := m.waitForRuntimeStart(ctx, containerID, startupTimeout)
	if err != nil {
		return CandidateResult{}, fmt.Errorf("step 6 (wait for runtime start) failed: %w", err)
	}

	// Step 7: Perform health checks
	primaryIP := detail.IPAddress
	if primaryIP == "" {
		for _, ip := range detail.Networks {
			if ip != "" {
				primaryIP = ip
				break
			}
		}
	}

	if err := EvaluateHealth(ctx, m.client, containerID, primaryIP, spec.HealthPolicy); err != nil {
		return CandidateResult{}, fmt.Errorf("step 7 (health checks) failed: %w", err)
	}

	// Step 8: Report candidate READY only after policy satisfied
	readyDetail, err := m.client.InspectContainer(ctx, containerID)
	if err != nil {
		readyDetail = detail
	}

	res := CandidateResult{
		Status:         "READY",
		ContainerID:    containerID,
		ContainerName:  containerName,
		Image:          spec.Image,
		ImageID:        imgDetail.ID,
		IPAddress:      primaryIP,
		NetworkIPs:     readyDetail.Networks,
		HealthStatus:   "healthy",
		StartedAt:      time.Now(),
		VerifiedLabels: readyDetail.Labels,
		Observation:    fmt.Sprintf("candidate %s ready and healthy", containerName),
		Duration:       time.Since(startTime),
	}
	if readyDetail.StartedAt != nil {
		res.StartedAt = *readyDetail.StartedAt
	}

	m.log.Info("candidate deployment ready",
		slog.String("containerId", containerID),
		slog.String("containerName", containerName),
		slog.Duration("duration", res.Duration),
	)

	return res, nil
}

func (m *Manager) ensureImage(ctx context.Context, spec CandidateSpec) (docker.ImageDetail, error) {
	if spec.Image == "" {
		return docker.ImageDetail{}, errors.New("image reference cannot be empty")
	}

	pullPolicy := spec.PullPolicy
	if pullPolicy == "" {
		pullPolicy = PullIfNotPresent
	}

	// If pull policy is not PullAlways, check local image cache
	if pullPolicy != PullAlways {
		img, err := m.client.InspectImage(ctx, spec.Image)
		if err == nil {
			return img, nil
		}
		if pullPolicy == PullNever {
			return docker.ImageDetail{}, fmt.Errorf("image %q not found locally and PullNever policy specified: %w", spec.Image, err)
		}
	}

	// Pull image
	m.log.Info("pulling image for candidate", slog.String("ref", spec.Image))
	pullRes, err := m.client.PullImageWithOptions(ctx, docker.PullImageOptions{
		Ref:          spec.Image,
		RegistryAuth: spec.RegistryAuth,
	})
	if err != nil {
		return docker.ImageDetail{}, fmt.Errorf("failed to pull image %q: %w", spec.Image, err)
	}

	img, err := m.client.InspectImage(ctx, spec.Image)
	if err != nil {
		// Fallback to synthetic detail from pull result if inspect fails
		return docker.ImageDetail{
			ID:       pullRes.ImageID,
			RepoTags: []string{spec.Image},
			Size:     pullRes.Size,
		}, nil
	}
	return img, nil
}

func (m *Manager) ensureNetworks(ctx context.Context, spec CandidateSpec) error {
	netMeta := network.Metadata{
		OrganizationID: spec.Metadata.OrganizationID,
		ProjectSlug:    spec.Metadata.AppShortID,
		EnvironmentID:  spec.Metadata.EnvironmentID,
	}

	for _, n := range spec.Networks {
		netName := strings.TrimSpace(n.Name)
		if netName == "" {
			continue
		}

		if m.networkMgr != nil {
			if _, err := m.networkMgr.EnsureNetwork(ctx, netName, netMeta, n.Internal); err != nil {
				return fmt.Errorf("network manager failed ensuring network %q: %w", netName, err)
			}
		} else {
			// Fallback direct check
			if _, err := m.client.InspectNetwork(ctx, netName); err != nil {
				_, createErr := m.client.CreateNetwork(ctx, docker.CreateNetworkRequest{
					Name:     netName,
					Internal: n.Internal,
					Driver:   "bridge",
					Labels:   netMeta.Labels(),
				})
				if createErr != nil {
					return fmt.Errorf("failed to create network %q: %w", netName, createErr)
				}
			}
		}
	}
	return nil
}

func (m *Manager) ensureVolumes(ctx context.Context, spec CandidateSpec) error {
	volMeta := volume.Metadata{
		OrganizationID: spec.Metadata.OrganizationID,
		ApplicationID:  spec.Metadata.ApplicationID,
	}

	for _, v := range spec.Volumes {
		volName := strings.TrimSpace(v.Name)
		if volName == "" {
			continue
		}

		if m.volumeMgr != nil {
			if _, err := m.volumeMgr.EnsureVolume(ctx, volName, volMeta, "local", nil); err != nil {
				return fmt.Errorf("volume manager failed ensuring volume %q: %w", volName, err)
			}
		} else {
			// Fallback direct check
			if _, err := m.client.InspectVolume(ctx, volName); err != nil {
				_, createErr := m.client.CreateVolume(ctx, docker.CreateVolumeRequest{
					Name:   volName,
					Driver: "local",
					Labels: volMeta.Labels(),
				})
				if createErr != nil {
					return fmt.Errorf("failed to create volume %q: %w", volName, createErr)
				}
			}
		}
	}
	return nil
}

func (m *Manager) createCandidateContainer(ctx context.Context, spec CandidateSpec) (string, string, error) {
	name, err := appcontainer.FormatName(spec.Metadata.AppShortID, spec.Metadata.RevisionID, spec.Metadata.Instance)
	if err != nil {
		return "", "", fmt.Errorf("failed to format platform container name: %w", err)
	}

	// Check if container already exists with this exact name
	existing, err := m.client.InspectContainer(ctx, name)
	if err == nil {
		// Existing container found: check if it's a candidate from a previous run
		isCand := existing.Labels != nil && existing.Labels[protocol.LabelCandidate] == "true"
		if !isCand {
			return "", "", fmt.Errorf("%w: container %q is an active non-candidate container", ErrStaleNonCandidate, name)
		}

		// It is a stale candidate container: clean it up safely
		m.log.Warn("cleaning up stale candidate container", slog.String("name", name), slog.String("id", existing.ID))
		_ = m.client.StopContainer(ctx, existing.ID, 5*time.Second)
		if removeErr := m.client.RemoveContainer(ctx, existing.ID, true); removeErr != nil {
			return "", "", fmt.Errorf("failed to remove stale candidate container %q: %w", name, removeErr)
		}
	}

	// Production traffic isolation:
	// 1. Exclude public proxy network (deploycore-proxy) from initial creation.
	//    The proxy network will only be connected upon activation in Phase A17.
	candidateNetworks := make([]string, 0, len(spec.Networks))
	for _, n := range spec.Networks {
		if strings.TrimSpace(n.Name) == "" {
			continue
		}
		if n.Name == protocol.ProxyNetworkName {
			// Do not connect proxy network during candidate evaluation
			continue
		}
		candidateNetworks = append(candidateNetworks, n.Name)
	}

	// 2. Prepare volume mounts
	volumeMounts := make([]docker.VolumeMount, 0, len(spec.Volumes))
	for _, v := range spec.Volumes {
		if v.Name != "" && v.ContainerPath != "" {
			volumeMounts = append(volumeMounts, docker.VolumeMount{
				VolumeName: v.Name,
				MountPath:  v.ContainerPath,
				ReadOnly:   v.ReadOnly,
			})
		}
	}

	// 3. Traefik labels may be present on the candidate, but the proxy network is
	//    withheld until activation. Traefik only routes containers on deploycore-proxy,
	//    so labels alone do not expose production traffic during health evaluation.
	var candidateTraefik *docker.TraefikConfig
	if spec.Traefik != nil {
		copyT := *spec.Traefik
		candidateTraefik = &copyT
	}

	// Build appcontainer spec
	appSpec := appcontainer.Spec{
		Metadata:       spec.Metadata,
		Image:          spec.Image,
		Entrypoint:     spec.Entrypoint,
		Command:        spec.Command,
		Env:            spec.Env,
		InternalPorts:  spec.InternalPorts,
		CPUMillis:      spec.CPUMillis,
		MemoryBytes:    spec.MemoryBytes,
		RestartPolicy:  spec.RestartPolicy,
		Networks:       candidateNetworks,
		Volumes:        volumeMounts,
		Traefik:        candidateTraefik,
		ReadOnlyRootFS: spec.ReadOnlyRootFS,
		Labels:         spec.Labels,
		Policy:         spec.Policy,
	}

	req, err := appcontainer.BuildCreateRequest(appSpec)
	if err != nil {
		return "", "", fmt.Errorf("failed to build container create request: %w", err)
	}

	// Explicitly confirm candidate label
	if req.PlatformLabels == nil {
		req.PlatformLabels = make(map[string]string)
	}
	req.PlatformLabels[protocol.LabelCandidate] = "true"

	createRes, err := m.client.CreateContainer(ctx, req)
	if err != nil {
		return "", "", fmt.Errorf("failed to create candidate container: %w", err)
	}

	return createRes.ID, name, nil
}

func (m *Manager) waitForRuntimeStart(ctx context.Context, containerID string, timeout time.Duration) (docker.ContainerDetail, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		detail, err := m.client.InspectContainer(waitCtx, containerID)
		if err == nil {
			if detail.State.Running {
				return detail, nil
			}

			// Check for premature exit or dead state
			if detail.State.Dead || detail.State.Status == "exited" || detail.State.ExitCode != 0 {
				return detail, fmt.Errorf("%w: container exited with code %d (status: %s, error: %s)",
					ErrPrematureExit, detail.State.ExitCode, detail.State.Status, detail.State.Error)
			}
		}

		select {
		case <-waitCtx.Done():
			return docker.ContainerDetail{}, fmt.Errorf("%w: exceeded %v waiting for container %s to run", ErrStartupTimeout, timeout, containerID)
		case <-ticker.C:
		}
	}
}
