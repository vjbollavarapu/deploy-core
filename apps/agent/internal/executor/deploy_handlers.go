package executor

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/appcontainer"
	"github.com/deploycore/deploy-core/apps/agent/internal/candidate"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/apps/agent/internal/volume"
)

var slugSafeRE = regexp.MustCompile(`[^a-z0-9\-]+`)

// candidateVolumePayload represents a volume mount in deployment instructions.
type candidateVolumePayload struct {
	Name          string `json:"name"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly,omitempty"`
}

// deployRevisionPayload is the JSON shape expected for protocol.OpDeployRevision.
type deployRevisionPayload struct {
	Phase           string                   `json:"phase,omitempty"` // "candidate", "create_container", etc.
	Trigger         string                   `json:"trigger,omitempty"`
	OrganizationID  string                   `json:"organizationId,omitempty"`
	ApplicationID   string                   `json:"applicationId"`
	DeploymentID    string                   `json:"deploymentId"`
	RevisionID      string                   `json:"revisionId"`
	ReplicaIndex    int                      `json:"replicaIndex"`
	Instance        int                      `json:"instance,omitempty"`
	ApplicationSlug string                   `json:"applicationSlug,omitempty"`
	EnvironmentID   string                   `json:"environmentId,omitempty"`
	Image           string                   `json:"image"`
	PullPolicy      string                   `json:"pullPolicy,omitempty"`
	Networks        []string                 `json:"networks,omitempty"`
	Volumes         []candidateVolumePayload `json:"volumes,omitempty"`
	InternalPorts   []docker.PortMapping     `json:"internalPorts,omitempty"`
	CPUMillis       int64                    `json:"cpuMillis,omitempty"`
	MemoryBytes     int64                    `json:"memoryBytes,omitempty"`
	RestartPolicy   string                   `json:"restartPolicy,omitempty"`
	Env             []string                 `json:"env,omitempty"`
	Traefik         *docker.TraefikConfig    `json:"traefik,omitempty"`
	HealthPolicy    *candidate.HealthPolicy  `json:"healthPolicy,omitempty"`
	StartupTimeout  string                   `json:"startupTimeout,omitempty"`
}

// deployRevisionHandler executes the candidate deployment primitive for OpDeployRevision.
func deployRevisionHandler(cli *docker.Client, log *slog.Logger) Handler {
	if log == nil {
		log = slog.Default()
	}

	netMgr := network.NewManager(cli)
	volMgr := volume.NewManager(cli)
	candMgr := candidate.NewManager(cli, netMgr, volMgr, log)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p deployRevisionPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		if p.Phase == "enable_routing" || p.Phase == "activate" {
			actHandler := activateRevisionHandler(cli, log)
			return actHandler.Execute(ctx, payload)
		}

		if p.Phase == "rollback" || p.Trigger == "rollback" {
			rbHandler := rollbackRevisionHandler(cli, log)
			return rbHandler.Execute(ctx, payload)
		}

		if strings.TrimSpace(p.ApplicationID) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "applicationId is required")
		}
		if strings.TrimSpace(p.Image) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "image is required")
		}

		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		// Defaults for deployment and revision identifiers if omitted
		if p.DeploymentID == "" {
			p.DeploymentID = "dep-initial"
		}
		if p.RevisionID == "" {
			p.RevisionID = "r1"
		}
		if p.EnvironmentID == "" {
			p.EnvironmentID = "env-default"
		}
		if p.OrganizationID == "" {
			p.OrganizationID = "org-default"
		}

		// Instance index (must be >= 1)
		instance := p.Instance
		if instance < 1 {
			if p.ReplicaIndex >= 0 {
				instance = p.ReplicaIndex + 1
			} else {
				instance = 1
			}
		}

		// Sanitize application slug for container naming standard
		appSlug := strings.ToLower(strings.TrimSpace(p.ApplicationSlug))
		if appSlug == "" {
			appSlug = strings.ToLower(strings.TrimSpace(p.ApplicationID))
		}
		appSlug = slugSafeRE.ReplaceAllString(appSlug, "")
		if len(appSlug) > 30 {
			appSlug = appSlug[:30]
		}
		if appSlug == "" {
			appSlug = "app"
		}

		// Parse startup timeout
		var startupTimeout time.Duration
		if p.StartupTimeout != "" {
			if d, err := time.ParseDuration(p.StartupTimeout); err == nil && d > 0 {
				startupTimeout = d
			}
		}

		// Prepare networks
		var netSpecs []candidate.NetworkSpec
		for _, netName := range p.Networks {
			if strings.TrimSpace(netName) != "" {
				netSpecs = append(netSpecs, candidate.NetworkSpec{Name: netName})
			}
		}

		// Prepare volumes
		var volSpecs []candidate.VolumeSpec
		for _, v := range p.Volumes {
			if strings.TrimSpace(v.Name) != "" && strings.TrimSpace(v.ContainerPath) != "" {
				volSpecs = append(volSpecs, candidate.VolumeSpec{
					Name:          v.Name,
					ContainerPath: v.ContainerPath,
					ReadOnly:      v.ReadOnly,
				})
			}
		}

		// Default health policy to ContainerState if unspecified
		hp := candidate.HealthPolicy{
			Type: candidate.HealthTypeContainerState,
		}
		if p.HealthPolicy != nil {
			hp = *p.HealthPolicy
		}

		candSpec := candidate.CandidateSpec{
			Metadata: appcontainer.Metadata{
				OrganizationID: p.OrganizationID,
				ApplicationID:  p.ApplicationID,
				EnvironmentID:  p.EnvironmentID,
				DeploymentID:   p.DeploymentID,
				RevisionID:     p.RevisionID,
				Instance:       instance,
				AppShortID:     appSlug,
				IsCandidate:    true,
			},
			Image:          p.Image,
			PullPolicy:     candidate.PullPolicy(p.PullPolicy),
			Networks:       netSpecs,
			Volumes:        volSpecs,
			InternalPorts:  p.InternalPorts,
			CPUMillis:      p.CPUMillis,
			MemoryBytes:    p.MemoryBytes,
			RestartPolicy:  docker.RestartPolicy(p.RestartPolicy),
			Env:            p.Env,
			Traefik:        p.Traefik,
			HealthPolicy:   hp,
			StartupTimeout: startupTimeout,
		}

		res, err := candMgr.StartCandidate(ctx, candSpec)
		if err != nil {
			if errors.Is(err, candidate.ErrStaleNonCandidate) {
				return ExecutionResult{}, Errorf(ErrCodeConflict, "candidate container conflict: %v", err)
			}
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"status":         res.Status,
				"containerId":    res.ContainerID,
				"containerName":  res.ContainerName,
				"image":          res.Image,
				"imageId":        res.ImageID,
				"ipAddress":      res.IPAddress,
				"networkIPs":     res.NetworkIPs,
				"healthStatus":   res.HealthStatus,
				"startedAt":      res.StartedAt.Format(time.RFC3339),
				"verifiedLabels": res.VerifiedLabels,
				"observation":    res.Observation,
				"durationMs":     res.Duration.Milliseconds(),
			},
		}, nil
	})
}
