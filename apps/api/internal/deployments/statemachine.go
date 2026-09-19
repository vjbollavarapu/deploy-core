package deployments

import (
	"fmt"
	"slices"
)

// Deployment lifecycle statuses (B11).
const (
	StatusPending           = "PENDING"
	StatusQueued            = "QUEUED"
	StatusPreparing         = "PREPARING"
	StatusFetchingSource    = "FETCHING_SOURCE"
	StatusBuilding          = "BUILDING"
	StatusImageReady        = "IMAGE_READY"
	StatusCreatingContainer = "CREATING_CONTAINER"
	StatusStarting          = "STARTING"
	StatusHealthChecking    = "HEALTH_CHECKING"
	StatusActivating        = "ACTIVATING"
	StatusRunning           = "RUNNING"

	StatusSourceFailed      = "SOURCE_FAILED"
	StatusBuildFailed       = "BUILD_FAILED"
	StatusImageFailed       = "IMAGE_FAILED"
	StatusContainerFailed   = "CONTAINER_FAILED"
	StatusStartFailed       = "START_FAILED"
	StatusHealthCheckFailed = "HEALTH_CHECK_FAILED"
	StatusRoutingFailed     = "ROUTING_FAILED"
	StatusCancelled         = "CANCELLED"
	StatusTimeout           = "TIMEOUT"
)

// Happy-path order (excluding terminal outcomes).
var happyPath = []string{
	StatusPending,
	StatusQueued,
	StatusPreparing,
	StatusFetchingSource,
	StatusBuilding,
	StatusImageReady,
	StatusCreatingContainer,
	StatusStarting,
	StatusHealthChecking,
	StatusActivating,
	StatusRunning,
}

// allowedTransitions maps from-status → allowed to-statuses.
var allowedTransitions = map[string][]string{
	StatusPending: {
		StatusQueued, StatusCancelled, StatusTimeout,
	},
	StatusQueued: {
		StatusPreparing, StatusCancelled, StatusTimeout,
	},
	StatusPreparing: {
		StatusFetchingSource, StatusCancelled, StatusTimeout,
	},
	StatusFetchingSource: {
		StatusBuilding, StatusSourceFailed, StatusCancelled, StatusTimeout,
	},
	StatusBuilding: {
		StatusImageReady, StatusBuildFailed, StatusImageFailed, StatusCancelled, StatusTimeout,
	},
	StatusImageReady: {
		StatusCreatingContainer, StatusImageFailed, StatusCancelled, StatusTimeout,
	},
	StatusCreatingContainer: {
		StatusStarting, StatusContainerFailed, StatusCancelled, StatusTimeout,
	},
	StatusStarting: {
		StatusHealthChecking, StatusStartFailed, StatusCancelled, StatusTimeout,
	},
	StatusHealthChecking: {
		StatusActivating, StatusHealthCheckFailed, StatusCancelled, StatusTimeout,
	},
	StatusActivating: {
		StatusRunning, StatusRoutingFailed, StatusCancelled, StatusTimeout,
	},
}

// IsTerminal reports whether status is immutable under normal operation.
func IsTerminal(status string) bool {
	switch status {
	case StatusRunning,
		StatusSourceFailed, StatusBuildFailed, StatusImageFailed, StatusContainerFailed,
		StatusStartFailed, StatusHealthCheckFailed, StatusRoutingFailed,
		StatusCancelled, StatusTimeout:
		return true
	default:
		return false
	}
}

// IsFailureTerminal reports terminal failure (not RUNNING success).
func IsFailureTerminal(status string) bool {
	return IsTerminal(status) && status != StatusRunning
}

// CanTransition reports whether from → to is a valid normal transition.
func CanTransition(from, to string) bool {
	if from == to {
		return false
	}
	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	return slices.Contains(allowed, to)
}

// ValidateTransition returns an error if the transition is not allowed.
func ValidateTransition(from, to string) error {
	if IsTerminal(from) {
		return fmt.Errorf("status %s is terminal", from)
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid transition %s → %s", from, to)
	}
	return nil
}

// NextHappyPath returns the next status on the success path, if any.
func NextHappyPath(from string) (string, bool) {
	for i, s := range happyPath {
		if s == from && i+1 < len(happyPath) {
			return happyPath[i+1], true
		}
	}
	return "", false
}
