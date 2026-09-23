package rollback

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/activation"
	"github.com/deploycore/deploy-core/apps/agent/internal/appcontainer"
	"github.com/deploycore/deploy-core/apps/agent/internal/candidate"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/apps/agent/internal/volume"
)

// Client defines the Docker methods required by the Rollback Executor.
type Client interface {
	candidate.Client
	activation.DockerClient
}

// Executor orchestrates rolling back to an immutable revision.
type Executor struct {
	client     Client
	networkMgr *network.Manager
	volumeMgr  *volume.Manager
	candMgr    *candidate.Manager
	log        *slog.Logger
}

// NewExecutor creates a new rollback Executor.
func NewExecutor(client Client, netMgr *network.Manager, volMgr *volume.Manager, log *slog.Logger) *Executor {
	if log == nil {
		log = slog.Default()
	}
	candMgr := candidate.NewManager(client, netMgr, volMgr, log)
	return &Executor{
		client:     client,
		networkMgr: netMgr,
		volumeMgr:  volMgr,
		candMgr:    candMgr,
		log:        log,
	}
}

// Execute performs rollback to an immutable revision according to Phase A19:
// 1. Validates that no build request is present (agent must not rebuild).
// 2. Verifies image exists locally or pulls by immutable digest.
// 3. Creates and starts the target candidate revision container.
// 4. Performs health checks.
//   - If target fails: traffic is NOT switched, failed candidate is cleaned, failure reported.
//
// 5. Activates target revision onto proxy network.
// 6. Drains and gracefully stops current active revision.
// 7. Returns structured rollback result.
func (e *Executor) Execute(ctx context.Context, spec RollbackSpec) (RollbackResult, error) {
	start := time.Now()

	// Guard: agent must not rebuild
	if spec.Dockerfile != "" || spec.SourceRepo != "" || spec.BuildContext != "" {
		return RollbackResult{}, ErrRebuildForbidden
	}

	if strings.TrimSpace(spec.ApplicationID) == "" {
		return RollbackResult{}, fmt.Errorf("applicationId is required")
	}
	if strings.TrimSpace(spec.TargetRevisionID) == "" {
		return RollbackResult{}, fmt.Errorf("targetRevisionId is required")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return RollbackResult{}, fmt.Errorf("image is required")
	}

	// Resolve immutable image reference
	imageRef := strings.TrimSpace(spec.Image)
	if spec.ImageDigest != "" && !strings.Contains(imageRef, "@") {
		imageRef = fmt.Sprintf("%s@%s", imageRef, spec.ImageDigest)
	}

	proxyNetwork := strings.TrimSpace(spec.ProxyNetwork)
	if proxyNetwork == "" {
		proxyNetwork = "deploycore-proxy"
	}

	instance := spec.Instance
	if instance < 1 {
		if spec.ReplicaIndex >= 0 {
			instance = spec.ReplicaIndex + 1
		} else {
			instance = 1
		}
	}

	appSlug := strings.ToLower(strings.TrimSpace(spec.ApplicationSlug))
	if appSlug == "" {
		appSlug = strings.ToLower(strings.TrimSpace(spec.ApplicationID))
	}
	if appSlug == "" {
		appSlug = "app"
	}

	orgID := strings.TrimSpace(spec.OrganizationID)
	if orgID == "" {
		orgID = "org-default"
	}
	envID := strings.TrimSpace(spec.EnvironmentID)
	if envID == "" {
		envID = "env-default"
	}
	depID := strings.TrimSpace(spec.DeploymentID)
	if depID == "" {
		depID = "dep-rollback"
	}

	// Prepare metadata for candidate
	meta := appcontainer.Metadata{
		OrganizationID: orgID,
		ApplicationID:  spec.ApplicationID,
		EnvironmentID:  envID,
		DeploymentID:   depID,
		RevisionID:     spec.TargetRevisionID,
		Instance:       instance,
		AppShortID:     appSlug,
		IsCandidate:    true,
	}

	expectedContainerName, _ := appcontainer.FormatName(meta.AppShortID, meta.RevisionID, meta.Instance)

	candSpec := candidate.CandidateSpec{
		Metadata:       meta,
		Image:          imageRef,
		PullPolicy:     candidate.PullIfNotPresent,
		Networks:       spec.Networks,
		Volumes:        spec.Volumes,
		InternalPorts:  spec.InternalPorts,
		CPUMillis:      spec.CPUMillis,
		MemoryBytes:    spec.MemoryBytes,
		RestartPolicy:  spec.RestartPolicy,
		Env:            spec.Env,
		Traefik:        spec.Traefik,
		HealthPolicy:   spec.HealthPolicy,
		StartupTimeout: spec.StartupTimeout,
	}

	e.log.Info("Starting rollback candidate deployment",
		"targetRevisionId", spec.TargetRevisionID,
		"image", imageRef,
		"expectedContainerName", expectedContainerName,
		"currentContainerId", spec.CurrentContainerID,
	)

	// Step 2-4: Create, start, and health-check target revision
	candRes, err := e.candMgr.StartCandidate(ctx, candSpec)
	if err != nil {
		e.log.Error("Rollback target candidate failed; cleaning up candidate and preserving current revision",
			"error", err,
			"candidateName", expectedContainerName,
		)

		// Failure safety: clean failed candidate container immediately
		e.cleanFailedCandidate(ctx, expectedContainerName)

		return RollbackResult{}, fmt.Errorf("%w: %v", ErrTargetHealthFailed, err)
	}

	// Step 5-6: Activate target and drain current active revision
	activator := activation.NewActivator(e.client, nil, e.log)
	actSpec := activation.ActivationSpec{
		CandidateContainerID:   candRes.ContainerID,
		CandidateContainerName: candRes.ContainerName,
		ProxyNetwork:           proxyNetwork,
		OldContainerID:         spec.CurrentContainerID,
		OldContainerName:       spec.CurrentContainerName,
		DrainDuration:          spec.DrainDuration,
		StopTimeout:            spec.TerminationTimeout,
		RetentionPolicy:        activation.RetentionPolicy(spec.RetentionPolicy),
	}

	actRes, err := activator.Activate(ctx, actSpec)
	if err != nil {
		e.log.Error("Activation failed during rollback; cleaning failed candidate",
			"error", err,
			"candidateId", candRes.ContainerID,
		)
		e.cleanFailedCandidate(ctx, candRes.ContainerID)
		return RollbackResult{}, fmt.Errorf("rollback activation failed: %w", err)
	}

	duration := time.Since(start)
	summary := fmt.Sprintf("Rollback to revision %s complete. Target %s active on %s. Previous container %s: %s",
		spec.TargetRevisionID, candRes.ContainerName, proxyNetwork, spec.CurrentContainerID, actRes.OldContainerStatus)

	return RollbackResult{
		Status:                 "COMPLETED",
		TargetContainerID:      candRes.ContainerID,
		TargetContainerName:    candRes.ContainerName,
		Image:                  candRes.Image,
		ImageDigest:            candRes.ImageID,
		HealthStatus:           candRes.HealthStatus,
		CurrentContainerID:     spec.CurrentContainerID,
		CurrentContainerStatus: actRes.OldContainerStatus,
		ActivatedAt:            actRes.ActivatedAt,
		DurationMs:             duration.Milliseconds(),
		Summary:                summary,
	}, nil
}

// cleanFailedCandidate stops and removes any failed candidate container so host remains clean.
func (e *Executor) cleanFailedCandidate(ctx context.Context, target string) {
	if target == "" {
		return
	}
	cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	insp, err := e.client.InspectContainer(cleanCtx, target)
	if err != nil {
		return // doesn't exist
	}

	if insp.State.Running {
		_ = e.client.StopContainer(cleanCtx, insp.ID, 0) // immediate SIGKILL
	}
	_ = e.client.RemoveContainer(cleanCtx, insp.ID, true)
}
