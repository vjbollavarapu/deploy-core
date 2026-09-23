package logs

import (
	"context"
	"time"
)

// StreamType represents the standard output or standard error stream.
type StreamType string

const (
	StreamStdout StreamType = "stdout"
	StreamStderr StreamType = "stderr"
)

// LogEntry represents a single demultiplexed, timestamped, and redacted log line.
type LogEntry struct {
	Stream    StreamType `json:"stream"`
	Message   string     `json:"message"`
	Timestamp time.Time  `json:"timestamp"`
}

// RedactionPolicy defines rules for stripping sensitive tokens from log streams.
type RedactionPolicy struct {
	SensitiveValues []string `json:"sensitiveValues,omitempty"` // Exact secrets/tokens to redact
	MinLength       int      `json:"minLength,omitempty"`       // Minimum string length to redact (default: 6)
	Mask            string   `json:"mask,omitempty"`            // Replacement mask (default: "[REDACTED]")
}

// StreamOptions specifies parameters for reading and following container logs.
type StreamOptions struct {
	ContainerID string           `json:"containerId"`
	ShowStdout  bool             `json:"showStdout"`
	ShowStderr  bool             `json:"showStderr"`
	Follow      bool             `json:"follow"`
	Since       string           `json:"since,omitempty"`
	Until       string           `json:"until,omitempty"`
	Tail        string           `json:"tail,omitempty"`
	Timestamps  bool             `json:"timestamps"`
	BufferSize  int              `json:"bufferSize,omitempty"` // Bounded buffer size (default: 500)
	Redaction   *RedactionPolicy `json:"redaction,omitempty"`
}

// LogSink is the destination interface for streamed log entries.
type LogSink interface {
	WriteEntry(ctx context.Context, entry LogEntry) error
}

// FuncSink adapts a function into a LogSink.
type FuncSink func(ctx context.Context, entry LogEntry) error

func (f FuncSink) WriteEntry(ctx context.Context, entry LogEntry) error {
	return f(ctx, entry)
}
