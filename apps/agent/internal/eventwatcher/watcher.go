package eventwatcher

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// EventSource defines the Docker event streaming contract.
type EventSource interface {
	DockerEvents(ctx context.Context, eventCh chan<- docker.Event, errCh chan<- error)
}

// Watcher subscribes to Docker daemon events, filters for DeployCore managed containers,
// translates relevant actions into platform events, and automatically reconnects on disconnects.
type Watcher struct {
	cli     EventSource
	handler EventHandler
	log     *slog.Logger

	mu      sync.Mutex
	stopCh  chan struct{}
	doneCh  chan struct{}
	running bool
}

// NewWatcher constructs a Watcher.
func NewWatcher(cli EventSource, handler EventHandler, log *slog.Logger) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{
		cli:     cli,
		handler: handler,
		log:     log,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start begins the event watching and automatic reconnection loop in the background.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	go w.supervisorLoop(ctx)
}

// Stop cleanly terminates the watcher.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	w.mu.Unlock()

	<-w.doneCh
}

// supervisorLoop maintains persistent event subscription with automatic reconnection.
func (w *Watcher) supervisorLoop(ctx context.Context) {
	defer close(w.doneCh)

	baseBackoff := 500 * time.Millisecond
	maxBackoff := 10 * time.Second
	currentBackoff := baseBackoff

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		eventCh := make(chan docker.Event, 100)
		errCh := make(chan error, 1)

		loopCtx, cancelLoop := context.WithCancel(ctx)

		// Start Docker event listener
		w.cli.DockerEvents(loopCtx, eventCh, errCh)

		// Process events until stream terminates
		reconnect := w.processEvents(loopCtx, eventCh, errCh)
		cancelLoop()

		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		if reconnect {
			// Add 20% jitter
			jitter := (rand.Float64()*0.4 + 0.8)
			sleepDur := time.Duration(float64(currentBackoff) * jitter)
			w.log.Info("docker event stream disconnected, reconnecting", slog.Duration("backoff", sleepDur))

			select {
			case <-time.After(sleepDur):
				currentBackoff *= 2
				if currentBackoff > maxBackoff {
					currentBackoff = maxBackoff
				}
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			}
		} else {
			currentBackoff = baseBackoff
		}
	}
}

// processEvents drains the event channel and translates matching events. Returns true if reconnect is needed.
func (w *Watcher) processEvents(ctx context.Context, eventCh <-chan docker.Event, errCh <-chan error) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-w.stopCh:
			return false
		case err, ok := <-errCh:
			if ok && err != nil {
				w.log.Warn("docker event stream encountered error", slog.String("error", err.Error()))
			}
			return true
		case ev, ok := <-eventCh:
			if !ok {
				// Channel closed, stream ended
				return true
			}

			// Translate and dispatch if relevant
			if pe, matched := TranslateEvent(ev); matched {
				if w.handler != nil {
					w.handler(pe)
				}
			}
		}
	}
}

// TranslateEvent parses and filters a raw Docker daemon event.
// Returns (PlatformEvent, true) if it matches a managed container and relevant action;
// otherwise (PlatformEvent{}, false).
func TranslateEvent(ev docker.Event) (PlatformEvent, bool) {
	// Only container events are relevant
	if ev.Type != "container" {
		return PlatformEvent{}, false
	}

	// Managed container filter: require deploycore.managed == "true" or name starting with "dc-"
	name := ev.Attrs["name"]
	cleanName := strings.TrimPrefix(name, "/")
	isManaged := ev.Attrs[protocol.LabelManaged] == "true" ||
		strings.HasPrefix(cleanName, protocol.ContainerPrefix+"-")

	if !isManaged {
		return PlatformEvent{}, false
	}

	// Determine relevant action
	rawAction := ev.Action
	var normalizedAction string
	var healthStatus string

	switch {
	case rawAction == "start":
		normalizedAction = "start"
	case rawAction == "stop":
		normalizedAction = "stop"
	case rawAction == "die":
		normalizedAction = "die"
	case rawAction == "restart":
		normalizedAction = "restart"
	case rawAction == "destroy" || rawAction == "kill" || rawAction == "oom":
		normalizedAction = "destroy"
	case strings.HasPrefix(rawAction, "health_status"):
		normalizedAction = "health_status"
		parts := strings.Split(rawAction, ":")
		if len(parts) > 1 {
			healthStatus = strings.TrimSpace(parts[1])
		}
	default:
		// Ignore irrelevant actions (e.g. exec_create, exec_start, attach, pause, unpause)
		return PlatformEvent{}, false
	}

	var exitCode *int
	if ecStr, ok := ev.Attrs["exitCode"]; ok {
		if ec, err := strconv.Atoi(ecStr); err == nil {
			exitCode = &ec
		}
	}

	appID := ev.Attrs[protocol.LabelApplicationID]
	revID := ev.Attrs[protocol.LabelRevisionID]

	eventID := fmt.Sprintf("evt_%s_%d", ev.ActorID, ev.Time.UnixNano())
	if ev.ActorID == "" {
		eventID = fmt.Sprintf("evt_%d", ev.Time.UnixNano())
	}

	pe := PlatformEvent{
		EventID:       eventID,
		Timestamp:     ev.Time,
		Action:        normalizedAction,
		ContainerID:   ev.ActorID,
		ContainerName: cleanName,
		ApplicationID: appID,
		RevisionID:    revID,
		ExitCode:      exitCode,
		HealthStatus:  healthStatus,
		Attributes:    ev.Attrs,
	}

	return pe, true
}
