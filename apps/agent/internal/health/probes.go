package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// probeHTTP executes an HTTP GET probe and validates the response status code.
func probeHTTP(ctx context.Context, host string, cfg Config) Observation {
	start := time.Now()
	obs := Observation{
		Timestamp: start,
	}

	if cfg.HTTPPort <= 0 {
		obs.Error = "http probe port must be > 0"
		obs.Message = "invalid port configuration"
		obs.Duration = time.Since(start)
		obs.DurationMs = obs.Duration.Milliseconds()
		return obs
	}

	scheme := strings.ToLower(strings.TrimSpace(cfg.HTTPScheme))
	if scheme == "" {
		scheme = "http"
	}

	path := strings.TrimSpace(cfg.HTTPPath)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	targetHost := host
	if targetHost == "" {
		targetHost = "127.0.0.1"
	}

	url := fmt.Sprintf("%s://%s%s", scheme, net.JoinHostPort(targetHost, strconv.Itoa(cfg.HTTPPort)), path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		obs.Error = fmt.Sprintf("failed to create request: %v", err)
		obs.Message = "invalid request"
		obs.Duration = time.Since(start)
		obs.DurationMs = obs.Duration.Milliseconds()
		return obs
	}

	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	obs.Duration = time.Since(start)
	obs.DurationMs = obs.Duration.Milliseconds()

	if err != nil {
		obs.Error = err.Error()
		obs.Message = fmt.Sprintf("request failed: %v", err)
		return obs
	}
	defer resp.Body.Close()

	obs.HTTPStatusCode = resp.StatusCode

	if matchExpectedStatus(resp.StatusCode, cfg.ExpectedStatus, cfg.ExpectedRange) {
		obs.Success = true
		obs.Message = fmt.Sprintf("HTTP %d OK", resp.StatusCode)
	} else {
		obs.Success = false
		obs.Error = fmt.Sprintf("unexpected HTTP status %d", resp.StatusCode)
		obs.Message = fmt.Sprintf("HTTP %d does not match expected criteria", resp.StatusCode)
	}

	return obs
}

// probeTCP tests socket reachability on target port.
func probeTCP(ctx context.Context, host string, port int) Observation {
	start := time.Now()
	obs := Observation{
		Timestamp: start,
	}

	if port <= 0 {
		obs.Error = "tcp probe port must be > 0"
		obs.Message = "invalid port configuration"
		obs.Duration = time.Since(start)
		obs.DurationMs = obs.Duration.Milliseconds()
		return obs
	}

	targetHost := host
	if targetHost == "" {
		targetHost = "127.0.0.1"
	}

	target := net.JoinHostPort(targetHost, strconv.Itoa(port))
	dialer := net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", target)
	obs.Duration = time.Since(start)
	obs.DurationMs = obs.Duration.Milliseconds()

	if err != nil {
		obs.Error = err.Error()
		obs.Message = fmt.Sprintf("connection to %s failed: %v", target, err)
		return obs
	}
	_ = conn.Close()

	obs.Success = true
	obs.Message = fmt.Sprintf("TCP %s reachable", target)
	return obs
}

// probeCommand executes a prevalidated command inside the container via Docker exec.
// CRITICAL: Executes strictly in-container, NEVER on the host machine.
func probeCommand(ctx context.Context, client DockerClient, containerID string, cmd []string) Observation {
	start := time.Now()
	obs := Observation{
		Timestamp: start,
	}

	if len(cmd) == 0 {
		obs.Error = "command probe array cannot be empty"
		obs.Message = "empty command"
		obs.Duration = time.Since(start)
		obs.DurationMs = obs.Duration.Milliseconds()
		return obs
	}

	res, err := client.ExecContainer(ctx, containerID, cmd)
	obs.Duration = time.Since(start)
	obs.DurationMs = obs.Duration.Milliseconds()
	obs.ExitCode = res.ExitCode

	if err != nil {
		obs.Error = err.Error()
		obs.Message = fmt.Sprintf("in-container exec failed: %v", err)
		return obs
	}

	if res.ExitCode == 0 {
		obs.Success = true
		out := strings.TrimSpace(res.Stdout)
		if out != "" {
			if len(out) > 100 {
				out = out[:100] + "..."
			}
			obs.Message = fmt.Sprintf("command succeeded: %s", out)
		} else {
			obs.Message = "command succeeded (exit 0)"
		}
	} else {
		obs.Success = false
		errText := strings.TrimSpace(res.Stderr)
		if errText == "" {
			errText = strings.TrimSpace(res.Stdout)
		}
		if len(errText) > 100 {
			errText = errText[:100] + "..."
		}
		obs.Error = fmt.Sprintf("exit code %d: %s", res.ExitCode, errText)
		obs.Message = fmt.Sprintf("command failed with exit code %d", res.ExitCode)
	}

	return obs
}

// probeContainerState verifies container runtime state and native Docker engine health status.
func probeContainerState(ctx context.Context, client DockerClient, containerID string) Observation {
	start := time.Now()
	obs := Observation{
		Timestamp: start,
	}

	detail, err := client.InspectContainer(ctx, containerID)
	obs.Duration = time.Since(start)
	obs.DurationMs = obs.Duration.Milliseconds()

	if err != nil {
		obs.Error = err.Error()
		obs.Message = fmt.Sprintf("container inspect failed: %v", err)
		return obs
	}

	obs.ExitCode = detail.State.ExitCode

	if detail.State.Dead {
		obs.Error = "container is dead"
		obs.Message = "dead state detected"
		return obs
	}

	if !detail.State.Running {
		obs.Error = fmt.Sprintf("container is not running (status: %s, exit code: %d)", detail.State.Status, detail.State.ExitCode)
		obs.Message = fmt.Sprintf("container stopped with code %d", detail.State.ExitCode)
		return obs
	}

	// If container has Docker HEALTHCHECK configured
	if detail.State.Health != nil && detail.State.Health.Status != "" && detail.State.Health.Status != "none" {
		switch detail.State.Health.Status {
		case "healthy":
			obs.Success = true
			obs.Message = "docker engine reports healthy"
		case "unhealthy":
			obs.Success = false
			obs.Error = fmt.Sprintf("docker health is unhealthy (streak: %d)", detail.State.Health.FailingStreak)
			obs.Message = "docker engine reports unhealthy"
		case "starting":
			obs.Success = false
			obs.Error = "docker health is starting"
			obs.Message = "docker healthcheck starting"
		default:
			obs.Success = false
			obs.Error = fmt.Sprintf("unknown docker health status: %s", detail.State.Health.Status)
			obs.Message = detail.State.Health.Status
		}
		return obs
	}

	obs.Success = true
	obs.Message = fmt.Sprintf("container running (PID %d)", detail.State.Pid)
	return obs
}

func matchExpectedStatus(statusCode int, expectedCode int, expectedRange string) bool {
	if expectedCode > 0 {
		return statusCode == expectedCode
	}

	if expectedRange != "" {
		// Supports "min-max" e.g. "200-299"
		if strings.Contains(expectedRange, "-") {
			parts := strings.Split(expectedRange, "-")
			if len(parts) == 2 {
				minVal, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
				maxVal, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err1 == nil && err2 == nil {
					return statusCode >= minVal && statusCode <= maxVal
				}
			}
		}

		// Supports comma-separated list e.g. "200,204,301"
		if strings.Contains(expectedRange, ",") {
			parts := strings.Split(expectedRange, ",")
			for _, p := range parts {
				if val, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && val == statusCode {
					return true
				}
			}
			return false
		}
	}

	// Default HTTP success: 200..399
	return statusCode >= 200 && statusCode < 400
}
