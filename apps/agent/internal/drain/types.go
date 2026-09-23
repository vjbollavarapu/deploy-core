package drain

import (
	"errors"
	"time"
)

var (
	// ErrContainerNotFound indicates the requested container does not exist.
	ErrContainerNotFound = errors.New("container not found")
	// ErrDrainCancelled indicates the drain wait was cancelled by context.
	ErrDrainCancelled = errors.New("drain operation cancelled")
)

// RetentionPolicy defines whether a decommissioned container is kept or removed.
type RetentionPolicy string

const (
	RetentionPolicyRetain RetentionPolicy = "retain"
	RetentionPolicyRemove RetentionPolicy = "remove"
)

// StopSpec defines the parameters for a graceful container stop and drain operation.
type StopSpec struct {
	ContainerID        string          `json:"containerId"`
	ContainerName      string          `json:"containerName,omitempty"`
	DrainRouting       bool            `json:"drainRouting"`                 // Whether to detach from proxy routing network before stopping
	ProxyNetwork       string          `json:"proxyNetwork,omitempty"`       // Network name (default: "deploycore-proxy")
	DrainDuration      time.Duration   `json:"drainDuration,omitempty"`      // Time to wait after route detachment (default: 5s if DrainRouting)
	TerminationTimeout time.Duration   `json:"terminationTimeout,omitempty"` // SIGTERM grace period before force kill (default: 15s)
	ForceKill          bool            `json:"forceKill,omitempty"`          // If true, immediately SIGKILL without graceful SIGTERM
	RetentionPolicy    RetentionPolicy `json:"retentionPolicy,omitempty"`    // "retain" (default) or "remove"
}

// StopResult defines the structured status output from a container stop operation.
type StopResult struct {
	ContainerID          string    `json:"containerId"`
	ContainerName        string    `json:"containerName,omitempty"`
	Status               string    `json:"status"` // "STOPPED", "ALREADY_STOPPED", "REMOVED"
	ExitCode             int       `json:"exitCode"`
	Drained              bool      `json:"drained"`
	DrainDurationMs      int64     `json:"drainDurationMs"`
	TerminationTimeoutMs int64     `json:"terminationTimeoutMs"`
	DurationMs           int64     `json:"durationMs"`
	Forced               bool      `json:"forced"`
	StoppedAt            time.Time `json:"stoppedAt"`
	Summary              string    `json:"summary"`
}
