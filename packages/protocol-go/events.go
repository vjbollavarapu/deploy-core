package protocol

import (
	"errors"
	"strings"
	"time"
)

// Platform Runtime Event Types
const (
	EventContainerStarted      = "container.started"
	EventContainerStopped      = "container.stopped"
	EventContainerDied         = "container.died"
	EventContainerRestarted    = "container.restarted"
	EventContainerDestroyed    = "container.destroyed"
	EventContainerHealthChange = "container.health_status"
	EventDeploymentStage       = "deployment.stage"
	EventBackupCompleted       = "backup.completed"
	EventRestoreCompleted      = "restore.completed"
)

// RuntimeEvent represents an asynchronous state change detected on the agent host.
type RuntimeEvent struct {
	Type          string         `json:"type"`
	ContainerID   string         `json:"containerId"`
	ContainerName string         `json:"containerName"`
	ApplicationID string         `json:"applicationId,omitempty"`
	RevisionID    string         `json:"revisionId,omitempty"`
	Status        string         `json:"status"`
	ExitCode      *int           `json:"exitCode,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
	Details       map[string]any `json:"details,omitempty"`
}

// Validate checks RuntimeEvent required fields.
func (e *RuntimeEvent) Validate() error {
	if strings.TrimSpace(e.Type) == "" {
		return errors.New("event type is required")
	}
	if strings.TrimSpace(e.ContainerID) == "" && strings.TrimSpace(e.ContainerName) == "" {
		return errors.New("containerId or containerName is required")
	}
	return nil
}

// BuildLogEvent represents an individual structured build output line with stage detection.
type BuildLogEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"` // stdout or stderr
	Message   string    `json:"message"`
	Stage     string    `json:"stage,omitempty"`
}

// Validate checks BuildLogEvent required fields.
func (b *BuildLogEvent) Validate() error {
	if strings.TrimSpace(b.Stream) == "" {
		return errors.New("stream (stdout/stderr) is required")
	}
	return nil
}
