package buildlogs

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// Streamer manages the incremental ingestion, stage tracking, bounded buffering,
// and Control Plane dispatch of build log events.
type Streamer struct {
	opts     StreamOptions
	sender   LogSender
	detector *StageDetector
	log      *slog.Logger

	eventCh chan BuildLogEvent

	mu       sync.Mutex
	retained []BuildLogEvent
	closed   bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewStreamer creates and starts a new build log Streamer.
func NewStreamer(ctx context.Context, opts StreamOptions, sender LogSender, log *slog.Logger) *Streamer {
	if log == nil {
		log = slog.Default()
	}

	bufSize := opts.BufferSize
	if bufSize <= 0 {
		bufSize = 1000
	} else if bufSize > 10000 {
		bufSize = 10000
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 25
	} else if batchSize > 200 {
		batchSize = 200
	}

	flushInterval := opts.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 200 * time.Millisecond
	}

	if opts.RetentionPolicy == "" {
		opts.RetentionPolicy = RetentionRetain
	}

	maxRetained := opts.MaxRetainedLines
	if maxRetained <= 0 {
		maxRetained = 200
	}

	opts.BufferSize = bufSize
	opts.BatchSize = batchSize
	opts.FlushInterval = flushInterval
	opts.MaxRetainedLines = maxRetained

	streamCtx, cancel := context.WithCancel(ctx)

	s := &Streamer{
		opts:     opts,
		sender:   sender,
		detector: NewStageDetector(),
		log:      log,
		eventCh:  make(chan BuildLogEvent, bufSize),
		retained: make([]BuildLogEvent, 0, maxRetained),
		ctx:      streamCtx,
		cancel:   cancel,
	}

	s.wg.Add(1)
	go s.flushLoop()

	return s
}

// Push ingests a raw docker build progress event, parses lines and stages,
// and buffers the resulting structured event.
func (s *Streamer) Push(ev docker.BuildProgressEvent) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	msg := ev.Stream
	streamName := "stdout"
	if msg == "" && ev.Error != "" {
		msg = ev.Error
		streamName = "stderr"
	}
	if strings.TrimSpace(msg) == "" {
		return
	}

	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	// Split by newline in case Docker sends multiple lines per event
	lines := strings.Split(msg, "\n")
	for _, rawLine := range lines {
		trimmed := strings.TrimRight(rawLine, "\r")
		if trimmed == "" && len(lines) > 1 {
			continue
		}

		stage, _ := s.detector.Detect(trimmed)

		event := BuildLogEvent{
			Timestamp: ts,
			Stream:    streamName,
			Message:   trimmed,
			Stage:     stage,
		}

		// Non-blocking bounded ring buffer: drop oldest if buffer full
		select {
		case s.eventCh <- event:
		default:
			select {
			case <-s.eventCh:
			default:
			}
			select {
			case s.eventCh <- event:
			default:
			}
		}
	}
}

// flushLoop runs in the background, grouping events into batches and dispatching
// incrementally to the Control Plane.
func (s *Streamer) flushLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.opts.FlushInterval)
	defer ticker.Stop()

	batch := make([]protocol.LogIngestLine, 0, s.opts.BatchSize)

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}

		if s.sender != nil {
			var depID *string
			if s.opts.DeploymentID != "" {
				depID = &s.opts.DeploymentID
			}

			req := protocol.LogIngestRequest{
				Kind:          "build",
				ApplicationID: s.opts.ApplicationID,
				DeploymentID:  depID,
				RevisionID:    s.opts.RevisionID,
				Entries:       batch,
			}

			_ = s.sender.SendLogs(s.ctx, req)
		}

		batch = make([]protocol.LogIngestLine, 0, s.opts.BatchSize)
	}

	for {
		select {
		case <-s.ctx.Done():
			// Context cancelled; flush what we have and exit
			flushBatch()
			return

		case <-ticker.C:
			flushBatch()

		case ev, ok := <-s.eventCh:
			if !ok {
				// Stream closed; flush remaining and exit
				flushBatch()
				return
			}

			// Record for retention policy
			s.recordRetained(ev)

			batch = append(batch, protocol.LogIngestLine{
				Stream:    ev.Stream,
				Message:   ev.Message,
				Timestamp: ev.Timestamp,
				Stage:     ev.Stage,
			})

			if len(batch) >= s.opts.BatchSize {
				flushBatch()
			}
		}
	}
}

func (s *Streamer) recordRetained(ev BuildLogEvent) {
	if s.opts.RetentionPolicy != RetentionRetain {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.retained) >= s.opts.MaxRetainedLines {
		// Drop oldest from retained ring
		s.retained = s.retained[1:]
	}
	s.retained = append(s.retained, ev)
}

// Close gracefully flushes all remaining events, stops the worker goroutine,
// and returns retained log events according to retention policy.
func (s *Streamer) Close() []BuildLogEvent {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.getRetainedCopy()
	}
	s.closed = true
	close(s.eventCh)
	s.mu.Unlock()

	// Wait for background flush to finish
	s.wg.Wait()
	s.cancel()

	return s.getRetainedCopy()
}

func (s *Streamer) getRetainedCopy() []BuildLogEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.opts.RetentionPolicy != RetentionRetain {
		return nil
	}

	out := make([]BuildLogEvent, len(s.retained))
	copy(out, s.retained)
	return out
}
