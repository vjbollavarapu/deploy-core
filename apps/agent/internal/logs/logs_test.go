package logs

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// Helper to construct an 8-byte Docker multiplexed frame
func makeDockerFrame(streamType byte, payload string) []byte {
	buf := make([]byte, 8+len(payload))
	buf[0] = streamType
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(payload)))
	copy(buf[8:], payload)
	return buf
}

func TestDemuxStream_StdoutAndStderr(t *testing.T) {
	var input bytes.Buffer
	// Frame 1: stdout "starting application\n"
	input.Write(makeDockerFrame(1, "starting application\n"))
	// Frame 2: stderr "warning: deprecated config\n"
	input.Write(makeDockerFrame(2, "warning: deprecated config\n"))

	var collected []LogEntry
	emit := func(entry LogEntry) {
		collected = append(collected, entry)
	}

	err := DemuxStream(&input, nil, false, emit)
	if err != nil {
		t.Fatalf("unexpected demux error: %v", err)
	}

	if len(collected) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(collected))
	}

	if collected[0].Stream != StreamStdout || collected[0].Message != "starting application" {
		t.Errorf("unexpected stdout entry: %+v", collected[0])
	}
	if collected[1].Stream != StreamStderr || collected[1].Message != "warning: deprecated config" {
		t.Errorf("unexpected stderr entry: %+v", collected[1])
	}
}

func TestDemuxStream_TimestampsParsing(t *testing.T) {
	var input bytes.Buffer
	tsStr := "2026-09-20T17:45:00.123456789Z"
	msg := tsStr + " server listening on :8080\n"
	input.Write(makeDockerFrame(1, msg))

	var collected []LogEntry
	emit := func(entry LogEntry) {
		collected = append(collected, entry)
	}

	err := DemuxStream(&input, nil, true, emit)
	if err != nil {
		t.Fatalf("unexpected demux error: %v", err)
	}

	if len(collected) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(collected))
	}

	expectedTime, _ := time.Parse(time.RFC3339Nano, tsStr)
	if !collected[0].Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, collected[0].Timestamp)
	}
	if collected[0].Message != "server listening on :8080" {
		t.Errorf("expected stripped message, got %q", collected[0].Message)
	}
}

func TestRedactor_PolicyAndLengthGuard(t *testing.T) {
	policy := &RedactionPolicy{
		SensitiveValues: []string{
			"very_secret_api_key_12345",
			"secret", // shorter substring
			"abc",    // < minLength (6) -> must NOT redact
		},
		MinLength: 6,
		Mask:      "[SECRET]",
	}

	redactor := NewRedactor(policy)

	input := "Connecting with very_secret_api_key_12345 and secret pass! abc remains intact."
	expected := "Connecting with [SECRET] and [SECRET] pass! abc remains intact."

	out := redactor.Redact(input)
	if out != expected {
		t.Errorf("expected redacted output %q, got %q", expected, out)
	}
}

type mockLogStreamClient struct {
	reader io.ReadCloser
}

func (m *mockLogStreamClient) OpenLogStream(ctx context.Context, id string, opts docker.LogOptions) (io.ReadCloser, error) {
	return m.reader, nil
}

type blockingReader struct {
	closed chan struct{}
}

func (b *blockingReader) Read(p []byte) (n int, err error) {
	<-b.closed
	return 0, io.EOF
}

func (b *blockingReader) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

func TestStreamer_CancellationAndNoLeak(t *testing.T) {
	reader := &blockingReader{closed: make(chan struct{})}
	mockCli := &mockLogStreamClient{reader: reader}
	streamer := NewStreamer(mockCli, nil)

	ctx, cancel := context.WithCancel(context.Background())

	var sinkEntries []LogEntry
	sink := FuncSink(func(ctx context.Context, entry LogEntry) error {
		sinkEntries = append(sinkEntries, entry)
		return nil
	})

	done := make(chan error, 1)
	go func() {
		opts := StreamOptions{
			ContainerID: "cont-1",
			Follow:      true,
			ShowStdout:  true,
			ShowStderr:  true,
		}
		done <- streamer.Stream(ctx, opts, sink)
	}()

	// Cancel context after brief moment
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("goroutine leak: streamer failed to terminate cleanly on context cancellation")
	}
}

func TestStreamer_BoundedRingBuffer_SlowConsumer(t *testing.T) {
	// Create a stream with 20 lines
	var input bytes.Buffer
	for i := 0; i < 20; i++ {
		input.Write(makeDockerFrame(1, strings.Repeat("a", 10)+"\n"))
	}

	mockCli := &mockLogStreamClient{reader: io.NopCloser(&input)}
	streamer := NewStreamer(mockCli, nil)

	var mu sync.Mutex
	var readCount int

	// Slow sink simulates consumer latency
	slowSink := FuncSink(func(ctx context.Context, entry LogEntry) error {
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		readCount++
		mu.Unlock()
		return nil
	})

	opts := StreamOptions{
		ContainerID: "cont-slow",
		BufferSize:  5, // small bounded buffer
		ShowStdout:  true,
	}

	err := streamer.Stream(context.Background(), opts, slowSink)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	mu.Lock()
	count := readCount
	mu.Unlock()

	if count == 0 {
		t.Errorf("expected entries to be read by slow sink")
	}
}
