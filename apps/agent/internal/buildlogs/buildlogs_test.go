package buildlogs

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestStageDetector_ClassicDockerAndBuildKit(t *testing.T) {
	detector := NewStageDetector()

	// Classic Docker step 1
	stage, isNew := detector.Detect("Step 1/3 : FROM alpine:3.18")
	if !isNew || stage != "Step 1/3: FROM alpine:3.18" {
		t.Errorf("expected new stage, got stage=%q isNew=%t", stage, isNew)
	}

	// Sub-line in step 1
	subStage, isNewSub := detector.Detect(" ---> 965ea09ff2eb")
	if isNewSub || subStage != "Step 1/3: FROM alpine:3.18" {
		t.Errorf("expected continuation of step 1, got stage=%q isNew=%t", subStage, isNewSub)
	}

	// Classic Docker step 2
	stage2, isNew2 := detector.Detect("Step 2/3 : RUN apk add --no-cache curl")
	if !isNew2 || stage2 != "Step 2/3: RUN apk add --no-cache curl" {
		t.Errorf("expected new stage 2, got stage=%q isNew=%t", stage2, isNew2)
	}

	// BuildKit stage
	bkStage, isNewBK := detector.Detect("#4 [internal] load build definition from Dockerfile")
	if !isNewBK || bkStage != "#4 [internal] load build definition from Dockerfile" {
		t.Errorf("expected BuildKit stage, got %q isNew=%t", bkStage, isNewBK)
	}

	// BuildKit export
	bkExport, isNewExp := detector.Detect("#8 exporting to image")
	if !isNewExp || bkExport != "#8 exporting to image" {
		t.Errorf("expected BuildKit export phase, got %q isNew=%t", bkExport, isNewExp)
	}
}

type mockLogSender struct {
	mu       sync.Mutex
	requests []protocol.LogIngestRequest
}

func (m *mockLogSender) SendLogs(ctx context.Context, req protocol.LogIngestRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	return nil
}

func TestStreamer_IncrementalBatchingAndRetention(t *testing.T) {
	sender := &mockLogSender{}

	opts := StreamOptions{
		ApplicationID:    "app-1",
		DeploymentID:     "dep-1",
		BufferSize:       100,
		BatchSize:        3,
		FlushInterval:    50 * time.Millisecond,
		RetentionPolicy:  RetentionRetain,
		MaxRetainedLines: 5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	streamer := NewStreamer(ctx, opts, sender, nil)

	// Push 7 lines
	for i := 1; i <= 7; i++ {
		streamer.Push(docker.BuildProgressEvent{
			Stream:    fmt.Sprintf("Step %d/7 : RUN echo %d\n", i, i),
			Timestamp: time.Now().UTC(),
		})
	}

	// Wait for batch flush
	time.Sleep(100 * time.Millisecond)

	retained := streamer.Close()

	sender.mu.Lock()
	reqCount := len(sender.requests)
	totalEntries := 0
	for _, r := range sender.requests {
		totalEntries += len(r.Entries)
	}
	sender.mu.Unlock()

	if reqCount < 2 {
		t.Errorf("expected incremental batch requests (at least 2 batches), got %d", reqCount)
	}
	if totalEntries != 7 {
		t.Errorf("expected 7 total entries sent, got %d", totalEntries)
	}

	// Retention policy: maxRetainedLines is 5
	if len(retained) != 5 {
		t.Errorf("expected 5 retained lines (bounded), got %d", len(retained))
	}
	if retained[len(retained)-1].Message != "Step 7/7 : RUN echo 7" {
		t.Errorf("expected latest line in retained, got %q", retained[len(retained)-1].Message)
	}
}

func TestStreamer_RetentionPolicyDiscard(t *testing.T) {
	sender := &mockLogSender{}

	opts := StreamOptions{
		ApplicationID:   "app-discard",
		RetentionPolicy: RetentionDiscard,
	}

	streamer := NewStreamer(context.Background(), opts, sender, nil)

	streamer.Push(docker.BuildProgressEvent{
		Stream:    "Step 1/1 : FROM alpine\n",
		Timestamp: time.Now().UTC(),
	})

	retained := streamer.Close()
	if len(retained) != 0 {
		t.Errorf("expected 0 retained lines under RetentionDiscard, got %d", len(retained))
	}
}

func TestStreamer_BoundedRingBuffer_SlowTransport(t *testing.T) {
	// Sender that simulates high latency
	slowSender := &mockLogSender{}

	opts := StreamOptions{
		ApplicationID: "app-slow",
		BufferSize:    10, // very small buffer
		BatchSize:     5,
		FlushInterval: 100 * time.Millisecond,
	}

	streamer := NewStreamer(context.Background(), opts, slowSender, nil)

	// Flood buffer with 50 events
	for i := 0; i < 50; i++ {
		streamer.Push(docker.BuildProgressEvent{
			Stream: fmt.Sprintf("line-%d\n", i),
		})
	}

	// Should not hang or deadlock
	streamer.Close()
}
