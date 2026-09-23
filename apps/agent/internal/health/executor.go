package health

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrThresholdExceeded indicates consecutive failures exceeded the configured threshold.
	ErrThresholdExceeded = errors.New("health check failure threshold exceeded")
	// ErrProbeTimeout indicates the overall health evaluation timed out.
	ErrProbeTimeout = errors.New("health check evaluation timed out")
)

// Execute runs the complete health evaluation lifecycle:
// initial delay -> periodic probe attempts -> consecutive success/failure accounting -> threshold determination.
func Execute(ctx context.Context, client DockerClient, containerID string, ipAddress string, cfg Config) (Result, error) {
	startTime := time.Now()

	// Normalize defaults
	probeType := cfg.ProbeType
	if probeType == "" {
		probeType = TypeContainerState
	}
	if probeType == TypeContainer {
		probeType = TypeContainerState
	}

	interval := cfg.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	successThreshold := cfg.SuccessThreshold
	if successThreshold <= 0 {
		successThreshold = 1
	}

	failureThreshold := cfg.FailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = 3
	}

	res := Result{
		Status:               StateStarting,
		ProbeType:            string(probeType),
		StartedAt:            startTime,
		Observations:         make([]Observation, 0),
		ConsecutiveSuccesses: 0,
		ConsecutiveFailures:  0,
	}

	// 1. Initial delay
	if cfg.InitialDelay > 0 {
		select {
		case <-time.After(cfg.InitialDelay):
		case <-ctx.Done():
			res.CompletedAt = time.Now()
			res.Duration = res.CompletedAt.Sub(startTime)
			res.DurationMs = res.Duration.Milliseconds()
			res.Summary = fmt.Sprintf("cancelled during initial delay: %v", ctx.Err())
			return res, ctx.Err()
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Execute first probe immediately after initial delay
	for {
		res.TotalChecks++
		obs := runProbe(ctx, client, containerID, ipAddress, probeType, cfg, timeout)
		res.Observations = append(res.Observations, obs)

		if obs.Success {
			res.ConsecutiveSuccesses++
			res.ConsecutiveFailures = 0

			if res.ConsecutiveSuccesses >= successThreshold {
				res.Status = StateHealthy
				res.Healthy = true
				res.CompletedAt = time.Now()
				res.Duration = res.CompletedAt.Sub(startTime)
				res.DurationMs = res.Duration.Milliseconds()
				res.Summary = fmt.Sprintf("%s health check satisfied (%d/%d consecutive successes): %s",
					probeType, res.ConsecutiveSuccesses, successThreshold, obs.Message)
				return res, nil
			}
		} else {
			res.ConsecutiveFailures++
			res.ConsecutiveSuccesses = 0

			if res.ConsecutiveFailures >= failureThreshold {
				res.Status = StateUnhealthy
				res.Healthy = false
				res.CompletedAt = time.Now()
				res.Duration = res.CompletedAt.Sub(startTime)
				res.DurationMs = res.Duration.Milliseconds()
				res.Summary = fmt.Sprintf("%s health check failed (%d consecutive failures, threshold %d): %s",
					probeType, res.ConsecutiveFailures, failureThreshold, obs.Error)
				return res, fmt.Errorf("%w: %s", ErrThresholdExceeded, res.Summary)
			}
		}

		select {
		case <-ctx.Done():
			res.Status = StateUnhealthy
			res.Healthy = false
			res.CompletedAt = time.Now()
			res.Duration = res.CompletedAt.Sub(startTime)
			res.DurationMs = res.Duration.Milliseconds()
			res.Summary = fmt.Sprintf("%v: %v", ErrProbeTimeout, ctx.Err())
			return res, fmt.Errorf("%w: %v", ErrProbeTimeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

func runProbe(ctx context.Context, client DockerClient, containerID string, ipAddress string, probeType ProbeType, cfg Config, timeout time.Duration) Observation {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch probeType {
	case TypeHTTP:
		return probeHTTP(probeCtx, ipAddress, cfg)
	case TypeTCP:
		return probeTCP(probeCtx, ipAddress, cfg.TCPPort)
	case TypeCommand:
		return probeCommand(probeCtx, client, containerID, cfg.Command)
	case TypeContainerState, TypeContainer, TypeDocker:
		return probeContainerState(probeCtx, client, containerID)
	default:
		start := time.Now()
		return Observation{
			Timestamp: start,
			Success:   false,
			Error:     fmt.Sprintf("unknown probe type %q", probeType),
			Message:   "unsupported probe type",
			Duration:  time.Since(start),
		}
	}
}
