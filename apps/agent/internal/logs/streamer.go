package logs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// DockerLogClient abstracts the log streaming method of the Docker engine.
type DockerLogClient interface {
	OpenLogStream(ctx context.Context, id string, opts docker.LogOptions) (io.ReadCloser, error)
}

// Streamer coordinates bounded streaming, demultiplexing, redaction, and clean cancellation.
type Streamer struct {
	client DockerLogClient
	log    *slog.Logger
}

// NewStreamer creates a new log Streamer.
func NewStreamer(client DockerLogClient, log *slog.Logger) *Streamer {
	if log == nil {
		log = slog.Default()
	}
	return &Streamer{
		client: client,
		log:    log,
	}
}

// Stream opens the container log stream, demultiplexes stdout/stderr, redacts sensitive
// tokens, manages a bounded ring buffer to prevent slow consumer bottlenecks, and flushes to sink.
func (s *Streamer) Stream(ctx context.Context, opts StreamOptions, sink LogSink) error {
	target := strings.TrimSpace(opts.ContainerID)
	if target == "" {
		return fmt.Errorf("containerId is required")
	}

	bufSize := opts.BufferSize
	if bufSize <= 0 {
		bufSize = 500
	} else if bufSize > 5000 {
		bufSize = 5000
	}

	tail := opts.Tail
	if tail == "" {
		tail = "100"
	}

	dockerOpts := docker.LogOptions{
		Follow:     opts.Follow,
		Since:      opts.Since,
		Until:      opts.Until,
		Tail:       tail,
		Timestamps: opts.Timestamps,
	}

	reader, err := s.client.OpenLogStream(ctx, target, dockerOpts)
	if err != nil {
		return fmt.Errorf("open log stream for container %q: %w", target, err)
	}
	defer reader.Close()

	redactor := NewRedactor(opts.Redaction)

	// Bounded channel to prevent unbounded memory usage on agent host
	entryCh := make(chan LogEntry, bufSize)

	// Slow consumer protection: drop oldest if buffer is completely saturated
	emit := func(entry LogEntry) {
		select {
		case entryCh <- entry:
		default:
			select {
			case <-entryCh: // discard oldest entry
			default:
			}
			select {
			case entryCh <- entry:
			default:
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Worker goroutine: demuxes stream and delivers to bounded channel
	go func() {
		defer wg.Done()
		defer close(entryCh)
		_ = DemuxStream(reader, redactor, opts.Timestamps, emit)
	}()

	// Consumer loop: forwards to sink, respects cancellation, and guarantees no goroutine leaks
	for {
		select {
		case <-ctx.Done():
			_ = reader.Close() // unblocks worker if waiting on reader
			wg.Wait()
			return ctx.Err()

		case entry, ok := <-entryCh:
			if !ok {
				wg.Wait()
				return nil
			}

			// Stream filtering
			if entry.Stream == StreamStdout && !opts.ShowStdout {
				continue
			}
			if entry.Stream == StreamStderr && !opts.ShowStderr {
				continue
			}

			if err := sink.WriteEntry(ctx, entry); err != nil {
				_ = reader.Close()
				wg.Wait()
				return err
			}
		}
	}
}
