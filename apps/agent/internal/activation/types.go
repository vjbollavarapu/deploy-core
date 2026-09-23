package activation

import (
	"errors"
	"time"
)

var (
	// ErrCandidateNotRunning indicates the candidate container is not running.
	ErrCandidateNotRunning = errors.New("candidate container is not running")
	// ErrCandidateUnhealthy indicates the candidate container is in an unhealthy state.
	ErrCandidateUnhealthy = errors.New("candidate container is unhealthy")
	// ErrRouteVerificationFailed indicates the post-activation route verification probe failed.
	ErrRouteVerificationFailed = errors.New("route verification probe failed")
)

// RetentionPolicy defines how the replaced revision container is handled after deactivation.
type RetentionPolicy string

const (
	// RetentionPolicyRetain preserves the stopped old container for fast instant rollback (default).
	RetentionPolicyRetain RetentionPolicy = "retain"
	// RetentionPolicyRemove removes the old container after draining and stopping.
	RetentionPolicyRemove RetentionPolicy = "remove"
)

// ActivationSpec defines parameters for a zero-downtime revision activation.
type ActivationSpec struct {
	CandidateContainerID      string          `json:"candidateContainerId"`
	CandidateContainerName    string          `json:"candidateContainerName,omitempty"`
	ProxyNetwork              string          `json:"proxyNetwork"` // e.g. "deploycore-proxy"
	VerifyRoute               bool            `json:"verifyRoute"`
	RouteVerifyURL            string          `json:"routeVerifyUrl,omitempty"`            // HTTP endpoint to probe before completing switch
	RouteVerifyTimeout        time.Duration   `json:"routeVerifyTimeout,omitempty"`        // probe timeout (default: 5s)
	RouteVerifyExpectedStatus int             `json:"routeVerifyExpectedStatus,omitempty"` // expected HTTP status code (default: 200..399)
	OldContainerID            string          `json:"oldContainerId,omitempty"`            // previous active revision container to drain and stop
	OldContainerName          string          `json:"oldContainerName,omitempty"`
	DrainDuration             time.Duration   `json:"drainDuration,omitempty"`   // time to wait after removing old from proxy (default: 5s)
	StopTimeout               time.Duration   `json:"stopTimeout,omitempty"`     // grace period for SIGTERM (default: 15s)
	RetentionPolicy           RetentionPolicy `json:"retentionPolicy,omitempty"` // "retain" (default) or "remove"
}

// ActivationResult defines the structured output of an activation operation.
type ActivationResult struct {
	Status                 string    `json:"status"` // "ACTIVATED"
	CandidateContainerID   string    `json:"candidateContainerId"`
	CandidateContainerName string    `json:"candidateContainerName,omitempty"`
	ProxyNetwork           string    `json:"proxyNetwork"`
	RouteVerified          bool      `json:"routeVerified"`
	OldContainerID         string    `json:"oldContainerId,omitempty"`
	OldContainerStatus     string    `json:"oldContainerStatus,omitempty"` // "drained_and_stopped", "drained_and_removed", or "none"
	DrainDurationMs        int64     `json:"drainDurationMs"`
	ActivatedAt            time.Time `json:"activatedAt"`
	Summary                string    `json:"summary"`
}
