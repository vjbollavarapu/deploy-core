package docker_test

import (
	"context"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

func TestBuildImage_NilContext(t *testing.T) {
	cfg := config.Config{}
	c, err := docker.NewClient(cfg)
	if err != nil {
		t.Skipf("skipping: cannot create docker client: %v", err)
	}

	_, err = c.BuildImage(context.Background(), docker.BuildImageOptions{
		Context: nil,
	})
	if err == nil {
		t.Fatal("expected error for nil build context, got nil")
	}
	ae, ok := err.(*docker.AgentError)
	if !ok {
		t.Fatalf("expected *docker.AgentError, got %T: %v", err, err)
	}
	if ae.Code != docker.ErrCodeImagePullFailed {
		t.Errorf("expected ErrCodeImagePullFailed, got %s", ae.Code)
	}
}

func TestBuildImage_OptionsTimeoutDefaults(t *testing.T) {
	opts := docker.BuildImageOptions{
		Dockerfile: "",
		Timeout:    0,
	}
	if opts.Dockerfile != "" {
		t.Errorf("expected empty initial dockerfile, got %q", opts.Dockerfile)
	}
	if opts.Timeout != 0 {
		t.Errorf("expected 0 initial timeout, got %v", opts.Timeout)
	}
}

func TestBuildImage_Cancellation(t *testing.T) {
	cfg := config.Config{}
	c, err := docker.NewClient(cfg)
	if err != nil {
		t.Skipf("skipping: cannot create docker client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// An empty context reader
	_, err = c.BuildImage(ctx, docker.BuildImageOptions{
		Context: nil,
	})
	if err == nil {
		t.Fatal("expected error for cancelled or nil context")
	}
}

func TestBuildProgressEvent(t *testing.T) {
	now := time.Now().UTC()
	ev := docker.BuildProgressEvent{
		Stream:    "Step 1/2 : FROM alpine\n",
		AuxID:     "sha256:123456",
		Timestamp: now,
	}
	if ev.Stream != "Step 1/2 : FROM alpine\n" {
		t.Errorf("unexpected Stream: %s", ev.Stream)
	}
	if ev.AuxID != "sha256:123456" {
		t.Errorf("unexpected AuxID: %s", ev.AuxID)
	}
	if ev.Timestamp != now {
		t.Errorf("unexpected Timestamp: %v", ev.Timestamp)
	}
}
