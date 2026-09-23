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
	// Set up environment for config loading
	os.Clearenv()
	os.Setenv("AGENT_CONTROL_PLANE_URL", "http://127.0.0.1:0") // Dummy URL to pass validation

	// We want to test graceful shutdown via context cancellation
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	a, err := agent.New(logger)
	if err != nil {
		t.Fatalf("expected no error creating agent, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run agent in a goroutine
	done := make(chan struct{})
	go func() {
		_ = a.Run(ctx)
		close(done)
	}()

	// Wait a brief moment, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for Run to return
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("agent failed to gracefully shut down within 2 seconds")
	}
}
