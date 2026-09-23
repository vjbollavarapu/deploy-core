package health

import (
	"context"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// ProbeType represents the category of health verification probe.
type ProbeType string

const (
	TypeHTTP           ProbeType = "HTTP"
	TypeTCP            ProbeType = "TCP"
	TypeCommand        ProbeType = "COMMAND"
	TypeContainerState ProbeType = "CONTAINER_STATE"
	TypeContainer      ProbeType = "CONTAINER" // alias for CONTAINER_STATE
	TypeDocker         ProbeType = "DOCKER"    // native Docker engine healthcheck
)

// State represents the calculated health evaluation status.
type State string

const (
	StateHealthy   State = "HEALTHY"
	StateUnhealthy State = "UNHEALTHY"
	StateStarting  State = "STARTING"
	StateUnknown   State = "UNKNOWN"
)

// DockerClient defines the Docker operations needed for health evaluation.
type DockerClient interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	ExecContainer(ctx context.Context, id string, cmd []string) (docker.ExecResult, error)
}

// Config defines the execution criteria for health check evaluation.
type Config struct {
	ProbeType        ProbeType     `json:"probeType"`
	InitialDelay     time.Duration `json:"initialDelay"`
	Interval         time.Duration `json:"interval"`
	Timeout          time.Duration `json:"timeout"`
	SuccessThreshold int           `json:"successThreshold"` // consecutive successes required
	FailureThreshold int           `json:"failureThreshold"` // consecutive failures before declaring failed

	// HTTP Probe settings
	HTTPScheme     string `json:"httpScheme,omitempty"` // "http" or "https"
	HTTPPort       int    `json:"httpPort,omitempty"`
	HTTPPath       string `json:"httpPath,omitempty"`
	ExpectedStatus int    `json:"expectedStatus,omitempty"` // e.g. 200 (0 = default 200..399)
	ExpectedRange  string `json:"expectedRange,omitempty"`  // e.g. "200-299" or "200,204"

	// TCP Probe settings
	TCPPort int `json:"tcpPort,omitempty"`

	// Command Probe settings (strictly in-container execution)
	Command []string `json:"command,omitempty"`
}

// Observation records a single probe sample execution.
type Observation struct {
	Timestamp      time.Time     `json:"timestamp"`
	Success        bool          `json:"success"`
	Duration       time.Duration `json:"duration"`
	DurationMs     int64         `json:"durationMs"`
	HTTPStatusCode int           `json:"httpStatusCode,omitempty"`
	ExitCode       int           `json:"exitCode,omitempty"`
	Message        string        `json:"message"`
	Error          string        `json:"error,omitempty"`
}

// Result is the structured output of health check evaluation.
type Result struct {
	Status               State         `json:"status"` // "HEALTHY", "UNHEALTHY", "STARTING"
	Healthy              bool          `json:"healthy"`
	ProbeType            string        `json:"probeType"`
	TotalChecks          int           `json:"totalChecks"`
	ConsecutiveSuccesses int           `json:"consecutiveSuccesses"`
	ConsecutiveFailures  int           `json:"consecutiveFailures"`
	Observations         []Observation `json:"observations"`
	Summary              string        `json:"summary"`
	StartedAt            time.Time     `json:"startedAt"`
	CompletedAt          time.Time     `json:"completedAt"`
	Duration             time.Duration `json:"duration"`
	DurationMs           int64         `json:"durationMs"`
}
