package agent_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/agent"
)

func TestAgent_Lifecycle(t *testing.T) {
	os.Clearenv()
	t.Setenv("AGENT_CONTROL_PLANE_URL", "http://127.0.0.1:0")
	t.Setenv("AGENT_DATA_DIR", t.TempDir())
	t.Setenv("AGENT_DOCKER_HOST", "unix:///tmp/deploycore-agent-test-no-docker.sock")

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	a, err := agent.New(logger)
	if err != nil {
		t.Fatalf("expected no error creating agent, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = a.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("agent failed to gracefully shut down within 2 seconds")
	}
}
