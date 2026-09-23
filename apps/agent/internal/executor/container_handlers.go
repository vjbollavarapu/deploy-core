package executor

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/drain"
)

// --------------------------------------------------------------------------
// Container operation handlers
// All handlers parse a strict typed struct from payload — no raw maps reach Docker.
// --------------------------------------------------------------------------

type stopContainerPayload struct {
	ContainerID        string `json:"containerId"`
	ContainerName      string `json:"containerName,omitempty"`
	TimeoutSeconds     int    `json:"timeoutSeconds,omitempty"`
	TerminationTimeout string `json:"terminationTimeout,omitempty"`
	DrainRouting       bool   `json:"drainRouting,omitempty"`
	ProxyNetwork       string `json:"proxyNetwork,omitempty"`
	DrainDuration      string `json:"drainDuration,omitempty"`
	ForceKill          bool   `json:"forceKill,omitempty"`
	Force              bool   `json:"force,omitempty"`
	RetentionPolicy    string `json:"retentionPolicy,omitempty"`
}

func stopContainerHandler(cli *docker.Client, log *slog.Logger) Handler {
	if log == nil {
		log = slog.Default()
	}

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p stopContainerPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		targetID := strings.TrimSpace(p.ContainerID)
		targetName := strings.TrimSpace(p.ContainerName)
		if targetID == "" && targetName == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "containerId or containerName is required")
		}

		forceKill := p.ForceKill || p.Force

		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		var termTimeout time.Duration
		if p.TerminationTimeout != "" {
			if d, err := time.ParseDuration(p.TerminationTimeout); err == nil && d > 0 {
				termTimeout = d
			}
		}
		if termTimeout <= 0 && p.TimeoutSeconds > 0 {
			termTimeout = time.Duration(p.TimeoutSeconds) * time.Second
		}

		var drainDuration time.Duration
		if p.DrainDuration != "" {
			if d, err := time.ParseDuration(p.DrainDuration); err == nil && d >= 0 {
				drainDuration = d
			}
		}

		spec := drain.StopSpec{
			ContainerID:        targetID,
			ContainerName:      targetName,
			DrainRouting:       p.DrainRouting,
			ProxyNetwork:       p.ProxyNetwork,
			DrainDuration:      drainDuration,
			TerminationTimeout: termTimeout,
			ForceKill:          forceKill,
			RetentionPolicy:    drain.RetentionPolicy(p.RetentionPolicy),
		}

		drainer := drain.NewDrainer(cli, log)
		res, err := drainer.Stop(ctx, spec)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"containerId":          res.ContainerID,
				"containerName":        res.ContainerName,
				"status":               res.Status,
				"stopped":              true,
				"exitCode":             res.ExitCode,
				"drained":              res.Drained,
				"drainDurationMs":      res.DrainDurationMs,
				"terminationTimeoutMs": res.TerminationTimeoutMs,
				"durationMs":           res.DurationMs,
				"forced":               res.Forced,
				"stoppedAt":            res.StoppedAt.Format(time.RFC3339),
				"summary":              res.Summary,
			},
		}, nil
	})
}

type startContainerPayload struct {
	ContainerID string `json:"containerId"`
}

func startContainerHandler(cli *docker.Client) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p startContainerPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		if p.ContainerID == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "containerId is required")
		}
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}
		// Idempotency: if container is already running, return success
		if detail, err := cli.InspectContainer(ctx, p.ContainerID); err == nil && detail.State.Running {
			return ExecutionResult{Output: map[string]any{"containerId": p.ContainerID, "started": true, "alreadyRunning": true}}, nil
		}
		if err := cli.StartContainer(ctx, p.ContainerID); err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}
		return ExecutionResult{Output: map[string]any{"containerId": p.ContainerID, "started": true}}, nil
	})
}

type restartContainerPayload struct {
	ContainerID    string `json:"containerId"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

func restartContainerHandler(cli *docker.Client) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p restartContainerPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		if p.ContainerID == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "containerId is required")
		}
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}
		timeout := time.Duration(p.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		if err := cli.RestartContainer(ctx, p.ContainerID, timeout); err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}
		return ExecutionResult{Output: map[string]any{"containerId": p.ContainerID, "restarted": true}}, nil
	})
}

type removeContainerPayload struct {
	ContainerID string `json:"containerId"`
	Force       bool   `json:"force"`
}

func removeContainerHandler(cli *docker.Client) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p removeContainerPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		if p.ContainerID == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "containerId is required")
		}
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}
		if err := cli.RemoveContainer(ctx, p.ContainerID, p.Force); err != nil {
			var ae *docker.AgentError
			if errors.As(err, &ae) && ae.Code == docker.ErrCodeNotFound {
				// Idempotency: already removed or non-existent
				return ExecutionResult{Output: map[string]any{"containerId": p.ContainerID, "removed": true, "notFound": true}}, nil
			}
			return ExecutionResult{}, wrapDockerErr(err)
		}
		return ExecutionResult{Output: map[string]any{"containerId": p.ContainerID, "removed": true}}, nil
	})
}

// wrapDockerErr converts a docker.AgentError into an ExecutionError.
func wrapDockerErr(err error) error {
	if ae, ok := err.(*docker.AgentError); ok {
		return &ExecutionError{Code: ErrCodeDockerError, Message: string(ae.Code) + ": " + ae.Message}
	}
	return &ExecutionError{Code: ErrCodeDockerError, Message: err.Error()}
}
