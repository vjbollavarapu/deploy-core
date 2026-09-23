package drain

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// DockerClient abstracts the engine calls required for graceful drain and stop.
type DockerClient interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	DisconnectNetwork(ctx context.Context, networkID string, containerID string, force bool) error
	StopContainer(ctx context.Context, id string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, id string, force bool) error
}

// Drainer coordinates traffic draining and graceful container shutdown.
type Drainer struct {
	client DockerClient
	log    *slog.Logger
}

// NewDrainer creates a new Drainer.
func NewDrainer(client DockerClient, log *slog.Logger) *Drainer {
	if log == nil {
		log = slog.Default()
	}
	return &Drainer{
		client: client,
		log:    log,
	}
}

// Stop executes graceful shutdown and optional traffic draining:
//  1. Inspects container; returns immediately if already stopped (idempotent).
//  2. If DrainRouting or connected to ProxyNetwork: detaches from proxy network and drains in-flight requests.
//  3. If ForceKill is true, issues immediate SIGKILL (timeout 0).
//     Otherwise, sends SIGTERM and waits up to TerminationTimeout before daemon force kills.
//  4. Inspects container to record final exit code and state.
//  5. If RetentionPolicy is "remove", removes the container.
func (d *Drainer) Stop(ctx context.Context, spec StopSpec) (StopResult, error) {
	start := time.Now()

	target := strings.TrimSpace(spec.ContainerID)
	if target == "" {
		target = strings.TrimSpace(spec.ContainerName)
	}
	if target == "" {
		return StopResult{}, fmt.Errorf("container ID or name is required")
	}

	proxyNetwork := strings.TrimSpace(spec.ProxyNetwork)
	if proxyNetwork == "" {
		proxyNetwork = "deploycore-proxy"
	}

	retention := spec.RetentionPolicy
	if retention == "" {
		retention = RetentionPolicyRetain
	}

	termTimeout := spec.TerminationTimeout
	if termTimeout <= 0 && !spec.ForceKill {
		termTimeout = 15 * time.Second
	}

	// 1. Pre-stop inspect
	insp, err := d.client.InspectContainer(ctx, target)
	if err != nil {
		return StopResult{}, fmt.Errorf("inspect container %q: %w", target, err)
	}

	containerName := insp.Name
	if containerName == "" {
		containerName = spec.ContainerName
	}

	// Idempotency: already stopped
	if !insp.State.Running {
		d.log.Info("Container is already stopped", "containerId", insp.ID, "containerName", containerName)
		return StopResult{
			ContainerID:          insp.ID,
			ContainerName:        containerName,
			Status:               "ALREADY_STOPPED",
			ExitCode:             insp.State.ExitCode,
			Drained:              false,
			DrainDurationMs:      0,
			TerminationTimeoutMs: termTimeout.Milliseconds(),
			DurationMs:           time.Since(start).Milliseconds(),
			Forced:               false,
			StoppedAt:            time.Now().UTC(),
			Summary:              fmt.Sprintf("Container %s was already stopped (exit code %d)", containerName, insp.State.ExitCode),
		}, nil
	}

	// 2. Traffic routing drain
	var drained bool
	var drainDuration time.Duration

	// Check if container should be drained from proxy network
	_, hasProxyNet := insp.Networks[proxyNetwork]
	if spec.DrainRouting || hasProxyNet {
		d.log.Info("Detaching container from proxy network for traffic drain",
			"containerId", insp.ID,
			"proxyNetwork", proxyNetwork,
			"drainDuration", spec.DrainDuration,
		)
		if err := d.client.DisconnectNetwork(ctx, proxyNetwork, insp.ID, false); err != nil {
			d.log.Warn("Failed to disconnect container from proxy network", "containerId", insp.ID, "error", err)
		}
		drained = true
		drainDuration = spec.DrainDuration
		if drainDuration > 0 {
			select {
			case <-time.After(drainDuration):
			case <-ctx.Done():
				return StopResult{}, fmt.Errorf("%w: %v", ErrDrainCancelled, ctx.Err())
			}
		}
	}

	// 3. Stop container (SIGTERM or immediate SIGKILL)
	var forced bool
	var stopTimeout time.Duration
	if spec.ForceKill {
		forced = true
		stopTimeout = 0
		d.log.Info("Explicit force kill requested; sending immediate SIGKILL", "containerId", insp.ID)
	} else {
		forced = false
		stopTimeout = termTimeout
		d.log.Info("Gracefully stopping container with SIGTERM", "containerId", insp.ID, "terminationTimeout", termTimeout)
	}

	if err := d.client.StopContainer(ctx, insp.ID, stopTimeout); err != nil {
		return StopResult{}, fmt.Errorf("stop container %q: %w", insp.ID, err)
	}

	// 4. Post-stop inspect for exit code
	exitCode := 0
	if postInsp, err := d.client.InspectContainer(ctx, insp.ID); err == nil {
		exitCode = postInsp.State.ExitCode
	}

	// 5. Retention policy
	status := "STOPPED"
	if retention == RetentionPolicyRemove {
		d.log.Info("Removing container under retention policy remove", "containerId", insp.ID)
		if err := d.client.RemoveContainer(ctx, insp.ID, false); err != nil {
			d.log.Warn("Failed to remove stopped container", "containerId", insp.ID, "error", err)
		} else {
			status = "REMOVED"
		}
	}

	duration := time.Since(start)
	summary := fmt.Sprintf("Container %s %s (exit code %d, drained: %t, forced: %t)",
		containerName, strings.ToLower(status), exitCode, drained, forced)

	return StopResult{
		ContainerID:          insp.ID,
		ContainerName:        containerName,
		Status:               status,
		ExitCode:             exitCode,
		Drained:              drained,
		DrainDurationMs:      drainDuration.Milliseconds(),
		TerminationTimeoutMs: termTimeout.Milliseconds(),
		DurationMs:           duration.Milliseconds(),
		Forced:               forced,
		StoppedAt:            time.Now().UTC(),
		Summary:              summary,
	}, nil
}
