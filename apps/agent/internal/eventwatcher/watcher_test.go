package eventwatcher

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockEventSource struct {
	mu        sync.Mutex
	calls     int
	eventsChs []chan<- docker.Event
}

func (m *mockEventSource) DockerEvents(ctx context.Context, eventCh chan<- docker.Event, errCh chan<- error) {
	m.mu.Lock()
	m.calls++
	m.eventsChs = append(m.eventsChs, eventCh)
	m.mu.Unlock()
}

func (m *mockEventSource) push(ev docker.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.eventsChs {
		ch <- ev
	}
}

func (m *mockEventSource) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.eventsChs {
		close(ch)
	}
	m.eventsChs = nil
}

func TestTranslateEvent_ManagedOnly(t *testing.T) {
	// Unmanaged event
	unmanaged := docker.Event{
		Type:   "container",
		Action: "start",
		Attrs: map[string]string{
			"name": "random-container",
		},
	}
	if _, ok := TranslateEvent(unmanaged); ok {
		t.Errorf("expected unmanaged container to be filtered out")
	}

	// Non-container event
	imageEv := docker.Event{
		Type:   "image",
		Action: "pull",
		Attrs: map[string]string{
			protocol.LabelManaged: "true",
		},
	}
	if _, ok := TranslateEvent(imageEv); ok {
		t.Errorf("expected image event to be filtered out")
	}

	// Managed container event
	managed := docker.Event{
		Type:    "container",
		Action:  "start",
		ActorID: "cnt-123",
		Time:    time.Now().UTC(),
		Attrs: map[string]string{
			"name":                      "/dc-app-prod",
			protocol.LabelManaged:       "true",
			protocol.LabelApplicationID: "app-1",
			protocol.LabelRevisionID:    "rev-1",
		},
	}
	pe, ok := TranslateEvent(managed)
	if !ok {
		t.Fatalf("expected managed container to be accepted")
	}
	if pe.ContainerID != "cnt-123" {
		t.Errorf("expected cnt-123, got %s", pe.ContainerID)
	}
	if pe.Action != "start" {
		t.Errorf("expected 'start', got %s", pe.Action)
	}
	if pe.ContainerName != "dc-app-prod" {
		t.Errorf("expected 'dc-app-prod', got %s", pe.ContainerName)
	}
	if pe.ApplicationID != "app-1" {
		t.Errorf("expected 'app-1', got %s", pe.ApplicationID)
	}
}

func TestTranslateEvent_ActionsAndHealth(t *testing.T) {
	// Die with exit code
	dieEv := docker.Event{
		Type:    "container",
		Action:  "die",
		ActorID: "cnt-123",
		Attrs: map[string]string{
			"name":                "dc-app",
			protocol.LabelManaged: "true",
			"exitCode":            "137",
		},
	}
	pe, ok := TranslateEvent(dieEv)
	if !ok {
		t.Fatalf("expected die event to be accepted")
	}
	if pe.Action != "die" {
		t.Errorf("expected 'die', got %s", pe.Action)
	}
	if pe.ExitCode == nil || *pe.ExitCode != 137 {
		t.Errorf("expected exit code 137, got %v", pe.ExitCode)
	}

	// Health status event
	healthEv := docker.Event{
		Type:    "container",
		Action:  "health_status: healthy",
		ActorID: "cnt-123",
		Attrs: map[string]string{
			"name":                "dc-app",
			protocol.LabelManaged: "true",
		},
	}
	peHealth, ok := TranslateEvent(healthEv)
	if !ok {
		t.Fatalf("expected health event to be accepted")
	}
	if peHealth.Action != "health_status" {
		t.Errorf("expected 'health_status', got %s", peHealth.Action)
	}
	if peHealth.HealthStatus != "healthy" {
		t.Errorf("expected healthStatus 'healthy', got %s", peHealth.HealthStatus)
	}
}

func TestWatcher_SupervisorReconnect(t *testing.T) {
	src := &mockEventSource{}

	var received []PlatformEvent
	var mu sync.Mutex

	handler := func(ev PlatformEvent) {
		mu.Lock()
		received = append(received, ev)
		mu.Unlock()
	}

	watcher := NewWatcher(src, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher.Start(ctx)

	// Wait for initial connection
	time.Sleep(50 * time.Millisecond)

	src.push(docker.Event{
		Type:    "container",
		Action:  "start",
		ActorID: "c-1",
		Time:    time.Now().UTC(),
		Attrs: map[string]string{
			"name":                "dc-first",
			protocol.LabelManaged: "true",
		},
	})

	time.Sleep(50 * time.Millisecond)

	// Force disconnect by closing channel
	src.closeAll()

	// Wait for reconnect backoff to trigger
	time.Sleep(600 * time.Millisecond)

	src.mu.Lock()
	calls := src.calls
	src.mu.Unlock()

	if calls < 2 {
		t.Errorf("expected at least 2 connection attempts (reconnection), got %d", calls)
	}

	watcher.Stop()

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 event received, got %d", count)
	}
}
