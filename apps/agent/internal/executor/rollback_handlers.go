package executor

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/candidate"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/drain"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/apps/agent/internal/rollback"
	"github.com/deploycore/deploy-core/apps/agent/internal/volume"
)

type rollbackRevisionPayload struct {
	OrganizationID       string                   `json:"organizationId,omitempty"`
	ApplicationID        string                   `json:"applicationId"`
	DeploymentID         string                   `json:"deploymentId,omitempty"`
	TargetRevisionID     string                   `json:"targetRevisionId,omitempty"`
	RevisionID           string                   `json:"revisionId,omitempty"` // alias
	ReplicaIndex         int                      `json:"replicaIndex"`
	Instance             int                      `json:"instance,omitempty"`
	ApplicationSlug      string                   `json:"applicationSlug,omitempty"`
	EnvironmentID        string                   `json:"environmentId,omitempty"`
	Image                string                   `json:"image"`
	ImageDigest          string                   `json:"imageDigest,omitempty"`
	Networks             []string                 `json:"networks,omitempty"`
	Volumes              []candidateVolumePayload `json:"volumes,omitempty"`
	InternalPorts        []docker.PortMapping     `json:"internalPorts,omitempty"`
	CPUMillis            int64                    `json:"cpuMillis,omitempty"`
	MemoryBytes          int64                    `json:"memoryBytes,omitempty"`
	RestartPolicy        string                   `json:"restartPolicy,omitempty"`
	Env                  []string                 `json:"env,omitempty"`
	Traefik              *docker.TraefikConfig    `json:"traefik,omitempty"`
	HealthPolicy         *candidate.HealthPolicy  `json:"healthPolicy,omitempty"`
	StartupTimeout       string                   `json:"startupTimeout,omitempty"`
	ProxyNetwork         string                   `json:"proxyNetwork,omitempty"`
	CurrentContainerID   string                   `json:"currentContainerId,omitempty"`
	CurrentContainerName string                   `json:"currentContainerName,omitempty"`
	DrainDuration        string                   `json:"drainDuration,omitempty"`
	TerminationTimeout   string                   `json:"terminationTimeout,omitempty"`
	RetentionPolicy      string                   `json:"retentionPolicy,omitempty"`
	// Rebuild rejection fields
	Dockerfile   string `json:"dockerfile,omitempty"`
	SourceRepo   string `json:"sourceRepo,omitempty"`
	BuildContext string `json:"buildContext,omitempty"`
}

// rollbackRevisionHandler executes the rollback primitive for OpRollbackRevision.
func rollbackRevisionHandler(cli *docker.Client, log *slog.Logger) Handler {
	if log == nil {
		log = slog.Default()
	}

	netMgr := network.NewManager(cli)
	volMgr := volume.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p rollbackRevisionPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		// Guard: rebuild forbidden during rollback
		if p.Dockerfile != "" || p.SourceRepo != "" || p.BuildContext != "" {
			return ExecutionResult{}, Errorf(ErrCodeValidation, "%v", rollback.ErrRebuildForbidden)
		}

		if strings.TrimSpace(p.ApplicationID) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "applicationId is required")
		}

		targetRev := strings.TrimSpace(p.TargetRevisionID)
		if targetRev == "" {
			targetRev = strings.TrimSpace(p.RevisionID)
		}
		if targetRev == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "targetRevisionId or revisionId is required")
		}

		if strings.TrimSpace(p.Image) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "image is required")
		}

		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		var startupTimeout time.Duration
		if p.StartupTimeout != "" {
			if d, err := time.ParseDuration(p.StartupTimeout); err == nil && d > 0 {
				startupTimeout = d
			}
		}

		var drainDuration time.Duration
		if p.DrainDuration != "" {
			if d, err := time.ParseDuration(p.DrainDuration); err == nil && d >= 0 {
				drainDuration = d
			}
		}

		var termTimeout time.Duration
		if p.TerminationTimeout != "" {
			if d, err := time.ParseDuration(p.TerminationTimeout); err == nil && d > 0 {
				termTimeout = d
			}
		}

		var netSpecs []candidate.NetworkSpec
		for _, netName := range p.Networks {
			if strings.TrimSpace(netName) != "" {
				netSpecs = append(netSpecs, candidate.NetworkSpec{Name: netName})
			}
		}

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

		hp := candidate.HealthPolicy{
			Type: candidate.HealthTypeContainerState,
		}
		if p.HealthPolicy != nil {
			hp = *p.HealthPolicy
		}

		spec := rollback.RollbackSpec{
			OrganizationID:       p.OrganizationID,
			ApplicationID:        p.ApplicationID,
			DeploymentID:         p.DeploymentID,
			TargetRevisionID:     targetRev,
			ReplicaIndex:         p.ReplicaIndex,
			Instance:             p.Instance,
			ApplicationSlug:      p.ApplicationSlug,
			EnvironmentID:        p.EnvironmentID,
			Image:                p.Image,
			ImageDigest:          p.ImageDigest,
			Networks:             netSpecs,
			Volumes:              volSpecs,
			InternalPorts:        p.InternalPorts,
			CPUMillis:            p.CPUMillis,
			MemoryBytes:          p.MemoryBytes,
			RestartPolicy:        docker.RestartPolicy(p.RestartPolicy),
			Env:                  p.Env,
			Traefik:              p.Traefik,
			HealthPolicy:         hp,
			StartupTimeout:       startupTimeout,
			ProxyNetwork:         p.ProxyNetwork,
			CurrentContainerID:   p.CurrentContainerID,
			CurrentContainerName: p.CurrentContainerName,
			DrainDuration:        drainDuration,
			TerminationTimeout:   termTimeout,
			RetentionPolicy:      drain.RetentionPolicy(p.RetentionPolicy),
			Dockerfile:           p.Dockerfile,
			SourceRepo:           p.SourceRepo,
			BuildContext:         p.BuildContext,
		}

		executor := rollback.NewExecutor(cli, netMgr, volMgr, log)
		res, err := executor.Execute(ctx, spec)
		if err != nil {
			if errors.Is(err, rollback.ErrRebuildForbidden) || errors.Is(err, rollback.ErrTargetHealthFailed) {
				return ExecutionResult{}, Errorf(ErrCodeValidation, "%v", err)
			}
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"status":                 res.Status,
				"targetContainerId":      res.TargetContainerID,
				"targetContainerName":    res.TargetContainerName,
				"image":                  res.Image,
				"imageDigest":            res.ImageDigest,
				"healthStatus":           res.HealthStatus,
				"currentContainerId":     res.CurrentContainerID,
				"currentContainerStatus": res.CurrentContainerStatus,
				"activatedAt":            res.ActivatedAt.Format(time.RFC3339),
				"durationMs":             res.DurationMs,
				"summary":                res.Summary,
			},
		}, nil
	})
}
