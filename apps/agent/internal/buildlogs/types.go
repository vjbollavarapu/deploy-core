package buildlogs

import (
	"context"
	"time"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// RetentionPolicy defines how build logs are kept in memory upon build completion.
type RetentionPolicy string

const (
	// RetentionRetain keeps a bounded tail of build logs in memory for debugging.
	RetentionRetain RetentionPolicy = "retain"
	// RetentionDiscard immediately frees build log events from memory once flushed.
	RetentionDiscard RetentionPolicy = "discard"
)

// BuildLogEvent represents a single structured build log entry.
type BuildLogEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"` // "stdout" or "stderr"
	Message   string    `json:"message"`
	Stage     string    `json:"stage,omitempty"`
}

// StreamOptions configures incremental build log collection and streaming.
type StreamOptions struct {
	ApplicationID    string          `json:"applicationId"`
	DeploymentID     string          `json:"deploymentId,omitempty"`
	RevisionID       *string         `json:"revisionId,omitempty"`
	BufferSize       int             `json:"bufferSize,omitempty"`       // Max buffer capacity (default: 1000)
	BatchSize        int             `json:"batchSize,omitempty"`        // Lines per flush batch (default: 25)
	FlushInterval    time.Duration   `json:"flushInterval,omitempty"`    // Max interval before auto-flush (default: 200ms)
	RetentionPolicy  RetentionPolicy `json:"retentionPolicy,omitempty"`  // "retain" or "discard" (default: "retain")
	MaxRetainedLines int             `json:"maxRetainedLines,omitempty"` // Bounded lines to keep if retained (default: 200)
}

// LogSender abstracts the transport client interface for sending logs to Control Plane.
type LogSender interface {
	SendLogs(ctx context.Context, req protocol.LogIngestRequest) error
}
