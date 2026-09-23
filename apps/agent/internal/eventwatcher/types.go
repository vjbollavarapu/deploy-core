package eventwatcher

import (
	"time"
)

// PlatformEvent is a normalized, platform-owned event derived from a Docker container event.
type PlatformEvent struct {
	EventID       string            `json:"eventId"`
	Timestamp     time.Time         `json:"timestamp"`
	Action        string            `json:"action"` // start, stop, die, restart, destroy, health_status
	ContainerID   string            `json:"containerId"`
	ContainerName string            `json:"containerName"`
	ApplicationID string            `json:"applicationId,omitempty"`
	RevisionID    string            `json:"revisionId,omitempty"`
	ExitCode      *int              `json:"exitCode,omitempty"`
	HealthStatus  string            `json:"healthStatus,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

// EventHandler receives processed platform events.
type EventHandler func(event PlatformEvent)
