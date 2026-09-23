package activation

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// DockerClient abstracts the Docker engine operations required for activation.
type DockerClient interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	ConnectNetwork(ctx context.Context, networkID string, containerID string) error
	DisconnectNetwork(ctx context.Context, networkID string, containerID string, force bool) error
	StopContainer(ctx context.Context, id string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, id string, force bool) error
}

// RouteProber defines the interface for probing a route before completing activation.
type RouteProber interface {
	Probe(ctx context.Context, url string, timeout time.Duration, expectedStatus int) error
}

// DefaultRouteProber probes routes using standard HTTP GET requests.
type DefaultRouteProber struct {
	Client *http.Client
}

// Probe executes an HTTP GET request to verify routing availability.
func (p *DefaultRouteProber) Probe(ctx context.Context, targetURL string, timeout time.Duration, expectedStatus int) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return fmt.Errorf("create probe request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe request failed: %w", err)
	}
	defer resp.Body.Close()

	if expectedStatus > 0 {
		if resp.StatusCode != expectedStatus {
			return fmt.Errorf("unexpected HTTP status %d, expected %d", resp.StatusCode, expectedStatus)
		}
	} else if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("unhealthy HTTP status %d", resp.StatusCode)
	}

	return nil
}

// Activator manages zero-downtime activation of candidate revisions.
type Activator struct {
	client DockerClient
	prober RouteProber
	log    *slog.Logger
}

// NewActivator creates a new revision Activator.
func NewActivator(client DockerClient, prober RouteProber, log *slog.Logger) *Activator {
	if prober == nil {
		prober = &DefaultRouteProber{}
	}
	if log == nil {
		log = slog.Default()
	}
	return &Activator{
		client: client,
		prober: prober,
		log:    log,
	}
}

// Activate performs zero-downtime revision activation:
// 1. Verifies candidate is running and healthy.
// 2. Connects candidate to the proxy network (idempotent).
// 3. Probes route if configured; rolls back candidate network attachment on failure without touching old revision.
// 4. Disconnects old revision from proxy network.
// 5. Waits for drain duration to complete in-flight traffic.
// 6. Stops old revision gracefully.
// 7. Retains or removes old revision container according to retention policy.
func (a *Activator) Activate(ctx context.Context, spec ActivationSpec) (ActivationResult, error) {
	candidateID := spec.CandidateContainerID
	if candidateID == "" {
		candidateID = spec.CandidateContainerName
	}
	if candidateID == "" {
		return ActivationResult{}, fmt.Errorf("candidate container ID or name is required")
	}

	proxyNetwork := spec.ProxyNetwork
	if strings.TrimSpace(proxyNetwork) == "" {
		proxyNetwork = "deploycore-proxy"
	}

	retentionPolicy := spec.RetentionPolicy
	if retentionPolicy == "" {
		retentionPolicy = RetentionPolicyRetain
	}

	stopTimeout := spec.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = 15 * time.Second
	}

	// 1. Verify candidate is running and healthy
	insp, err := a.client.InspectContainer(ctx, candidateID)
	if err != nil {
		return ActivationResult{}, fmt.Errorf("inspect candidate %q: %w", candidateID, err)
	}
	if !insp.State.Running {
		return ActivationResult{}, ErrCandidateNotRunning
	}
	if insp.State.Health != nil && insp.State.Health.Status == "unhealthy" {
		return ActivationResult{}, ErrCandidateUnhealthy
	}

	candidateName := insp.Name
	if candidateName == "" {
		candidateName = spec.CandidateContainerName
	}

	a.log.Info("Activating candidate container",
		"candidateId", insp.ID,
		"candidateName", candidateName,
		"proxyNetwork", proxyNetwork,
		"oldContainerId", spec.OldContainerID,
	)

	// 2. Attach / enable routing on proxy network
	if err := a.client.ConnectNetwork(ctx, proxyNetwork, insp.ID); err != nil {
		return ActivationResult{}, fmt.Errorf("connect candidate to proxy network %q: %w", proxyNetwork, err)
	}

	// 3. Verify route if configured
	if spec.VerifyRoute && spec.RouteVerifyURL != "" {
		a.log.Info("Probing candidate route before deactivating old revision",
			"url", spec.RouteVerifyURL,
			"timeout", spec.RouteVerifyTimeout,
		)
		probeErr := a.prober.Probe(ctx, spec.RouteVerifyURL, spec.RouteVerifyTimeout, spec.RouteVerifyExpectedStatus)
		if probeErr != nil {
			a.log.Error("Route verification failed; rolling back candidate network attachment",
				"error", probeErr,
				"candidateId", insp.ID,
			)
			// Immediate rollback: disconnect candidate from proxy network so old revision remains serving
			_ = a.client.DisconnectNetwork(ctx, proxyNetwork, insp.ID, false)
			return ActivationResult{}, fmt.Errorf("%w: %v", ErrRouteVerificationFailed, probeErr)
		}
	}

	// 4. Handle old revision deactivation if present
	oldStatus := "none"
	oldID := strings.TrimSpace(spec.OldContainerID)
	if oldID == "" {
		oldID = strings.TrimSpace(spec.OldContainerName)
	}

	if oldID != "" && oldID != insp.ID {
		a.log.Info("Draining and decommissioning old revision container",
			"oldContainerId", oldID,
			"drainDuration", spec.DrainDuration,
			"retentionPolicy", retentionPolicy,
		)

		// 4a. Disconnect old revision from proxy network so new traffic ceases
		if err := a.client.DisconnectNetwork(ctx, proxyNetwork, oldID, false); err != nil {
			a.log.Warn("Failed to disconnect old container from proxy network", "oldContainerId", oldID, "error", err)
		}

		// 4b. Drain period to let in-flight connections complete
		if spec.DrainDuration > 0 {
			select {
			case <-time.After(spec.DrainDuration):
			case <-ctx.Done():
				return ActivationResult{}, ctx.Err()
			}
		}

		// 4c. Graceful stop (SIGTERM)
		if err := a.client.StopContainer(ctx, oldID, stopTimeout); err != nil {
			a.log.Warn("Failed to stop old container gracefully", "oldContainerId", oldID, "error", err)
		}

		// 4d. Retention policy handling
		if retentionPolicy == RetentionPolicyRemove {
			if err := a.client.RemoveContainer(ctx, oldID, false); err != nil {
				a.log.Warn("Failed to remove old container under remove policy", "oldContainerId", oldID, "error", err)
			}
			oldStatus = "drained_and_removed"
		} else {
			oldStatus = "drained_and_stopped"
		}
	}

	summary := fmt.Sprintf("Candidate %s activated onto %s. Old container %s: %s",
		candidateName, proxyNetwork, oldID, oldStatus)

	return ActivationResult{
		Status:                 "ACTIVATED",
		CandidateContainerID:   insp.ID,
		CandidateContainerName: candidateName,
		ProxyNetwork:           proxyNetwork,
		RouteVerified:          spec.VerifyRoute,
		OldContainerID:         oldID,
		OldContainerStatus:     oldStatus,
		DrainDurationMs:        spec.DrainDuration.Milliseconds(),
		ActivatedAt:            time.Now().UTC(),
		Summary:                summary,
	}, nil
}
