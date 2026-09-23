package docker_test

import (
	"context"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

func TestRegistryAuth_Zero(t *testing.T) {
	auth := &docker.RegistryAuth{
		Username:      "myuser",
		Password:      "supersecretpassword",
		ServerAddress: "https://index.docker.io/v1/",
		IdentityToken: "id-token-123",
		RegistryToken: "reg-token-456",
	}

	auth.Zero()

	if auth.Password != "" {
		t.Errorf("expected Password to be cleared, got %q", auth.Password)
	}
	if auth.IdentityToken != "" {
		t.Errorf("expected IdentityToken to be cleared, got %q", auth.IdentityToken)
	}
	if auth.RegistryToken != "" {
		t.Errorf("expected RegistryToken to be cleared, got %q", auth.RegistryToken)
	}
	// Username and ServerAddress are not secrets and may be retained
	if auth.Username != "myuser" {
		t.Errorf("expected Username to remain, got %q", auth.Username)
	}
	if auth.ServerAddress != "https://index.docker.io/v1/" {
		t.Errorf("expected ServerAddress to remain, got %q", auth.ServerAddress)
	}
}

func TestPullImage_EmptyReference(t *testing.T) {
	cfg := config.Config{}
	c, err := docker.NewClient(cfg)
	if err != nil {
		t.Skipf("skipping: cannot create docker client: %v", err)
	}

	_, err = c.PullImageWithOptions(context.Background(), docker.PullImageOptions{
		Ref: "",
	})
	if err == nil {
		t.Fatal("expected error for empty image reference, got nil")
	}
	ae, ok := err.(*docker.AgentError)
	if !ok {
		t.Fatalf("expected *docker.AgentError, got %T: %v", err, err)
	}
	if ae.Code != docker.ErrCodeImagePullFailed {
		t.Errorf("expected ErrCodeImagePullFailed, got %s", ae.Code)
	}
}

func TestPullImage_LivePublicRegistry(t *testing.T) {
	// Integration test that runs only if Docker is available
	c := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var events []docker.PullProgressEvent
	progressFn := func(ev docker.PullProgressEvent) {
		events = append(events, ev)
	}

	// Pull small image
	res, err := c.PullImageWithOptions(ctx, docker.PullImageOptions{
		Ref:        "alpine:3.19",
		ProgressFn: progressFn,
	})
	if err != nil {
		t.Fatalf("failed to pull image: %v", err)
	}

	if res.Status != "pulled" && res.Status != "already_up_to_date" {
		t.Errorf("unexpected pull status: %s", res.Status)
	}
	if res.ImageID == "" {
		t.Error("expected non-empty image ID")
	}
	if res.Size <= 0 {
		t.Errorf("expected positive image size, got %d", res.Size)
	}
	if res.PullDuration <= 0 {
		t.Errorf("expected positive pull duration, got %v", res.PullDuration)
	}
}
