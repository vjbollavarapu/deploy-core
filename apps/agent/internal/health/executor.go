package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

const (
	// dockerStartingPoll is the inspect cadence while Docker reports "starting".
	dockerStartingPoll = time.Second
	// maxDockerStartingBudget caps the wait so a huge HEALTHCHECK cannot outlive
	// the agent command lifecycle.
	maxDockerStartingBudget = 10 * time.Minute
	dockerDefaultInterval   = 30 * time.Second
	dockerDefaultTimeout    = 30 * time.Second
	dockerDefaultRetries    = 3
)

var (
	// ErrThresholdExceeded indicates consecutive failures exceeded the configured threshold.
	ErrThresholdExceeded = errors.New("health check failure threshold exceeded")
	// ErrProbeTimeout indicates the overall health evaluation timed out.
	ErrProbeTimeout = errors.New("health check evaluation timed out")
	// ErrDockerStartingTimeout indicates Docker stayed in "starting" until the bounded deadline.
	ErrDockerStartingTimeout = errors.New("docker health remained starting until the startup deadline")
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

	var startingDeadline time.Time

	// Execute first probe immediately after initial delay
	for {
		if err := ctx.Err(); err != nil {
			return finishCancelled(res, startTime, err), fmt.Errorf("%w: %w", ErrProbeTimeout, err)
		}

		res.TotalChecks++
		obs := runProbe(ctx, client, containerID, ipAddress, probeType, cfg, timeout)
		res.Observations = append(res.Observations, obs)

		if obs.Pending {
			if startingDeadline.IsZero() {
				startingDeadline = dockerStartingDeadline(time.Now(), obs.dockerStart)
			}
			now := time.Now()
			if !now.Before(startingDeadline) {
				res.Status = StateUnhealthy
				res.Healthy = false
				res.CompletedAt = now
				res.Duration = res.CompletedAt.Sub(startTime)
				res.DurationMs = res.Duration.Milliseconds()
				res.Summary = ErrDockerStartingTimeout.Error()
				return res, fmt.Errorf("%w: %s", ErrDockerStartingTimeout, res.Summary)
			}
			wait := dockerStartingPoll
			if remaining := startingDeadline.Sub(now); remaining < wait {
				wait = remaining
			}
			if err := waitHealth(ctx, wait); err != nil {
				return finishCancelled(res, startTime, err), fmt.Errorf("%w: %w", ErrProbeTimeout, err)
			}
			continue
		}

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
			return finishCancelled(res, startTime, ctx.Err()), fmt.Errorf("%w: %w", ErrProbeTimeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

func finishCancelled(res Result, startTime time.Time, err error) Result {
	res.Status = StateUnhealthy
	res.Healthy = false
	res.CompletedAt = time.Now()
	res.Duration = res.CompletedAt.Sub(startTime)
	res.DurationMs = res.Duration.Milliseconds()
	res.Summary = fmt.Sprintf("%v: %v", ErrProbeTimeout, err)
	return res
}

func waitHealth(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// dockerStartingBudget is how long DeployCore waits for Docker to leave "starting".
// Docker keeps that status through StartPeriod and until Retries checks fail.
// Each check can consume Timeout and is spaced by Interval. One extra Interval
// covers scheduling delay. Zero interval, timeout, or retries use Docker's defaults.
func dockerStartingBudget(cfg *docker.HealthCheckConfig) time.Duration {
	interval := dockerDefaultInterval
	timeout := dockerDefaultTimeout
	retries := dockerDefaultRetries
	var start time.Duration
	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		if cfg.Retries > 0 {
			retries = cfg.Retries
		}
		if cfg.StartPeriod > 0 {
			start = cfg.StartPeriod
		}
	}
	if interval > maxDockerStartingBudget {
		interval = maxDockerStartingBudget
	}
	if timeout > maxDockerStartingBudget {
		timeout = maxDockerStartingBudget
	}
	if start > maxDockerStartingBudget {
		start = maxDockerStartingBudget
	}
	if retries > 100 {
		retries = 100
	}
	perCheck := interval + timeout
	budget := start + time.Duration(retries)*perCheck + interval
	if budget <= 0 || budget > maxDockerStartingBudget {
		return maxDockerStartingBudget
	}
	return budget
}

func dockerStartingDeadline(now time.Time, clock *dockerStartClock) time.Time {
	var cfg *docker.HealthCheckConfig
	origin := now
	if clock != nil {
		cfg = clock.Config
		if clock.StartedAt != nil && !clock.StartedAt.IsZero() && !clock.StartedAt.After(now) {
			origin = *clock.StartedAt
		}
	}
	deadline := origin.Add(dockerStartingBudget(cfg))
	capAt := now.Add(maxDockerStartingBudget)
	if deadline.After(capAt) {
		return capAt
	}
	return deadline
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
