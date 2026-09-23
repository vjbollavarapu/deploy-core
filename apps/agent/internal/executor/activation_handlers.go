package executor

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/activation"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// activateRevisionPayload defines the JSON payload schema for OpActivateRevision
// and phase="enable_routing" / "activate" operations.
type activateRevisionPayload struct {
	CandidateContainerID      string `json:"candidateContainerId,omitempty"`
	CandidateContainerName    string `json:"candidateContainerName,omitempty"`
	ContainerName             string `json:"containerName,omitempty"` // alias from orchestrator
	ContainerID               string `json:"containerId,omitempty"`   // alias
	ProxyNetwork              string `json:"proxyNetwork,omitempty"`
	VerifyRoute               bool   `json:"verifyRoute,omitempty"`
	RouteVerifyURL            string `json:"routeVerifyUrl,omitempty"`
	RouteVerifyTimeout        string `json:"routeVerifyTimeout,omitempty"`
	RouteVerifyExpectedStatus int    `json:"routeVerifyExpectedStatus,omitempty"`
	OldContainerID            string `json:"oldContainerId,omitempty"`
	OldContainerName          string `json:"oldContainerName,omitempty"`
	DrainDuration             string `json:"drainDuration,omitempty"`
	StopTimeout               string `json:"stopTimeout,omitempty"`
	RetentionPolicy           string `json:"retentionPolicy,omitempty"`
}

// activateRevisionHandler executes zero-downtime revision activation.
func activateRevisionHandler(cli *docker.Client, log *slog.Logger) Handler {
	if log == nil {
		log = slog.Default()
	}

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p activateRevisionPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		candidateID := strings.TrimSpace(p.CandidateContainerID)
		if candidateID == "" {
			candidateID = strings.TrimSpace(p.ContainerID)
		}
		candidateName := strings.TrimSpace(p.CandidateContainerName)
		if candidateName == "" {
			candidateName = strings.TrimSpace(p.ContainerName)
		}

		if candidateID == "" && candidateName == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "candidateContainerId or candidateContainerName is required")
		}

		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		// Parse timeouts & durations
		var routeTimeout time.Duration
		if p.RouteVerifyTimeout != "" {
			if d, err := time.ParseDuration(p.RouteVerifyTimeout); err == nil && d > 0 {
				routeTimeout = d
			}
		}

		var drainDuration time.Duration
		if p.DrainDuration != "" {
			if d, err := time.ParseDuration(p.DrainDuration); err == nil && d >= 0 {
				drainDuration = d
			}
		}

		var stopTimeout time.Duration
		if p.StopTimeout != "" {
			if d, err := time.ParseDuration(p.StopTimeout); err == nil && d > 0 {
				stopTimeout = d
			}
		}

		spec := activation.ActivationSpec{
			CandidateContainerID:      candidateID,
			CandidateContainerName:    candidateName,
			ProxyNetwork:              p.ProxyNetwork,
			VerifyRoute:               p.VerifyRoute,
			RouteVerifyURL:            p.RouteVerifyURL,
			RouteVerifyTimeout:        routeTimeout,
			RouteVerifyExpectedStatus: p.RouteVerifyExpectedStatus,
			OldContainerID:            p.OldContainerID,
			OldContainerName:          p.OldContainerName,
			DrainDuration:             drainDuration,
			StopTimeout:               stopTimeout,
			RetentionPolicy:           activation.RetentionPolicy(p.RetentionPolicy),
		}

		activator := activation.NewActivator(cli, nil, log)
		res, err := activator.Activate(ctx, spec)
		if err != nil {
			if errors.Is(err, activation.ErrCandidateNotRunning) || errors.Is(err, activation.ErrCandidateUnhealthy) {
				return ExecutionResult{}, Errorf(ErrCodeValidation, "%v", err)
			}
			if errors.Is(err, activation.ErrRouteVerificationFailed) {
				return ExecutionResult{}, Errorf(ErrCodeRoutingFailed, "%v", err)
			}
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"status":                 res.Status,
				"candidateContainerId":   res.CandidateContainerID,
				"candidateContainerName": res.CandidateContainerName,
				"proxyNetwork":           res.ProxyNetwork,
				"routeVerified":          res.RouteVerified,
				"oldContainerId":         res.OldContainerID,
				"oldContainerStatus":     res.OldContainerStatus,
				"drainDurationMs":        res.DrainDurationMs,
				"activatedAt":            res.ActivatedAt.Format(time.RFC3339),
				"summary":                res.Summary,
			},
		}, nil
	})
}
