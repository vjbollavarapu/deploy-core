package executor

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/logs"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// --------------------------------------------------------------------------
// Log operation handlers
// --------------------------------------------------------------------------

type fetchLogsPayload struct {
	ContainerID     string                `json:"containerId"`
	ContainerName   string                `json:"containerName,omitempty"`
	Tail            string                `json:"tail"`  // "all" or number string, default "200"
	Since           string                `json:"since"` // RFC3339 or relative (e.g. "5m")
	Until           string                `json:"until"`
	Timestamps      bool                  `json:"timestamps"`
	ShowStdout      *bool                 `json:"showStdout,omitempty"`
	ShowStderr      *bool                 `json:"showStderr,omitempty"`
	RedactionPolicy *logs.RedactionPolicy `json:"redaction,omitempty"`
}

// fetchLogsHandler collects logs non-streamingly and returns them in the result.
// Output is demultiplexed, timestamp-parsed, redacted, and bounded by Tail.
func fetchLogsHandler(cli *docker.Client, log *slog.Logger) Handler {
	if log == nil {
		log = slog.Default()
	}

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p fetchLogsPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		target := strings.TrimSpace(p.ContainerID)
		if target == "" {
			target = strings.TrimSpace(p.ContainerName)
		}
		if target == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "containerId or containerName is required")
		}

		tail := p.Tail
		if tail == "" {
			tail = "200"
		}

		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		showStdout := true
		if p.ShowStdout != nil {
			showStdout = *p.ShowStdout
		}
		showStderr := true
		if p.ShowStderr != nil {
			showStderr = *p.ShowStderr
		}

		opts := logs.StreamOptions{
			ContainerID: target,
			ShowStdout:  showStdout,
			ShowStderr:  showStderr,
			Follow:      false,
			Since:       p.Since,
			Until:       p.Until,
			Tail:        tail,
			Timestamps:  p.Timestamps,
			BufferSize:  500,
			Redaction:   p.RedactionPolicy,
		}

		streamer := logs.NewStreamer(cli, log)

		var collected []string
		sink := logs.FuncSink(func(ctx context.Context, entry logs.LogEntry) error {
			collected = append(collected, entry.Message)
			return nil
		})

		if err := streamer.Stream(ctx, opts, sink); err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"containerId": target,
			"logs":        strings.Join(collected, "\n"),
			"lineCount":   len(collected),
		}}, nil
	})
}

type streamLogsPayload struct {
	ContainerID     string                `json:"containerId"`
	ContainerName   string                `json:"containerName,omitempty"`
	ApplicationID   string                `json:"applicationId,omitempty"`
	DeploymentID    *string               `json:"deploymentId,omitempty"`
	RevisionID      *string               `json:"revisionId,omitempty"`
	Tail            string                `json:"tail"`
	Since           string                `json:"since"`
	Timestamps      bool                  `json:"timestamps"`
	ShowStdout      *bool                 `json:"showStdout,omitempty"`
	ShowStderr      *bool                 `json:"showStderr,omitempty"`
	BufferSize      int                   `json:"bufferSize,omitempty"`
	BatchSize       int                   `json:"batchSize,omitempty"`
	RedactionPolicy *logs.RedactionPolicy `json:"redaction,omitempty"`
}

// streamLogsHandler follows logs until context is cancelled, demultiplexes,
// redacts, and streams batched entries to the Control Plane via transport.Client.
func streamLogsHandler(cli *docker.Client, tr transport.Client, log *slog.Logger) Handler {
	if log == nil {
		log = slog.Default()
	}

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p streamLogsPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		target := strings.TrimSpace(p.ContainerID)
		if target == "" {
			target = strings.TrimSpace(p.ContainerName)
		}
		if target == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "containerId or containerName is required")
		}

		tail := p.Tail
		if tail == "" {
			tail = "100"
		}

		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		showStdout := true
		if p.ShowStdout != nil {
			showStdout = *p.ShowStdout
		}
		showStderr := true
		if p.ShowStderr != nil {
			showStderr = *p.ShowStderr
		}

		batchLimit := p.BatchSize
		if batchLimit <= 0 {
			batchLimit = 50
		}

		opts := logs.StreamOptions{
			ContainerID: target,
			ShowStdout:  showStdout,
			ShowStderr:  showStderr,
			Follow:      true,
			Since:       p.Since,
			Tail:        tail,
			Timestamps:  p.Timestamps,
			BufferSize:  p.BufferSize,
			Redaction:   p.RedactionPolicy,
		}

		streamer := logs.NewStreamer(cli, log)

		var batch []protocol.LogIngestLine
		var totalLines int64
		lastFlush := time.Now()

		flush := func(force bool) error {
			if len(batch) == 0 {
				return nil
			}
			if !force && len(batch) < batchLimit && time.Since(lastFlush) < 1*time.Second {
				return nil
			}

			if tr != nil {
				appID := p.ApplicationID
				if appID == "" {
					appID = target
				}
				req := protocol.LogIngestRequest{
					Kind:          "runtime",
					ApplicationID: appID,
					DeploymentID:  p.DeploymentID,
					RevisionID:    p.RevisionID,
					Entries:       batch,
				}
				_ = tr.SendLogs(ctx, req)
			}

			batch = batch[:0]
			lastFlush = time.Now()
			return nil
		}

		sink := logs.FuncSink(func(ctx context.Context, entry logs.LogEntry) error {
			batch = append(batch, protocol.LogIngestLine{
				Stream:    string(entry.Stream),
				Message:   entry.Message,
				Timestamp: entry.Timestamp,
			})
			totalLines++
			return flush(false)
		})

		err := streamer.Stream(ctx, opts, sink)
		_ = flush(true) // Flush remaining

		if err != nil && err != context.Canceled {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"containerId": target,
			"streamed":    true,
			"totalLines":  totalLines,
		}}, nil
	})
}
