package executor

import (
	"context"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/health"
)

type runHealthCheckPayload struct {
	ContainerID      string   `json:"containerId,omitempty"`
	ContainerName    string   `json:"containerName,omitempty"`
	ProbeType        string   `json:"probeType,omitempty"`
	Path             string   `json:"path,omitempty"`
	Port             *int     `json:"port,omitempty"`
	Scheme           string   `json:"scheme,omitempty"`
	ExpectedStatus   int      `json:"expectedStatus,omitempty"`
	ExpectedRange    string   `json:"expectedRange,omitempty"`
	Command          []string `json:"command,omitempty"`
	TimeoutSeconds   int      `json:"timeout,omitempty"`
	InitialDelaySecs int      `json:"initialDelaySeconds,omitempty"`
	IntervalSeconds  int      `json:"intervalSeconds,omitempty"`
	Retries          int      `json:"retries,omitempty"`
}

// runHealthCheckHandler executes structured health checks for protocol.OpRunHealthCheck.
func runHealthCheckHandler(cli *docker.Client) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p runHealthCheckPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		target := strings.TrimSpace(p.ContainerID)
		if target == "" {
			target = strings.TrimSpace(p.ContainerName)
		}
		if target == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "containerId or containerName is required")
		}

		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		detail, err := cli.InspectContainer(ctx, target)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		primaryIP := detail.IPAddress
		if primaryIP == "" {
			for _, ip := range detail.Networks {
				if ip != "" {
					primaryIP = ip
					break
				}
			}
		}

		var port int
		if p.Port != nil {
			port = *p.Port
		}

		timeout := time.Duration(p.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 3 * time.Second
		}

		initialDelay := time.Duration(p.InitialDelaySecs) * time.Second
		interval := time.Duration(p.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 500 * time.Millisecond
		}

		failureThreshold := p.Retries
		if failureThreshold <= 0 {
			failureThreshold = 3
		}

		cfg := health.Config{
			ProbeType:        health.ProbeType(p.ProbeType),
			InitialDelay:     initialDelay,
			Interval:         interval,
			Timeout:          timeout,
			SuccessThreshold: 1,
			FailureThreshold: failureThreshold,
			HTTPScheme:       p.Scheme,
			HTTPPort:         port,
			HTTPPath:         p.Path,
			ExpectedStatus:   p.ExpectedStatus,
			ExpectedRange:    p.ExpectedRange,
			TCPPort:          port,
			Command:          p.Command,
		}

		res, execErr := health.Execute(ctx, cli, detail.ID, primaryIP, cfg)

		output := map[string]any{
			"containerId":          detail.ID,
			"containerName":        detail.Name,
			"status":               string(res.Status),
			"healthy":              res.Healthy,
			"probeType":            res.ProbeType,
			"totalChecks":          res.TotalChecks,
			"consecutiveSuccesses": res.ConsecutiveSuccesses,
			"consecutiveFailures":  res.ConsecutiveFailures,
			"observations":         res.Observations,
			"summary":              res.Summary,
			"durationMs":           res.DurationMs,
		}

		if execErr != nil || !res.Healthy {
			return ExecutionResult{Output: output}, Errorf(ErrCodeDockerError, "%s", res.Summary)
		}

		return ExecutionResult{Output: output}, nil
	})
}
