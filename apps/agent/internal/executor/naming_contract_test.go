package executor

import (
	"context"
	"log/slog"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/appcontainer"
	"github.com/deploycore/deploy-core/apps/agent/internal/candidate"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

func TestDeployRevision_MissingRevisionNumberRejectsBeforeDocker(t *testing.T) {
	called := false
	prev := startDeployCandidate
	t.Cleanup(func() { startDeployCandidate = prev })
	startDeployCandidate = func(context.Context, *docker.Client, *slog.Logger, candidate.CandidateSpec) (candidate.CandidateResult, error) {
		called = true
		return candidate.CandidateResult{}, nil
	}
	h := deployRevisionHandler(&docker.Client{}, runtimeTransport{}, nil)
	for _, number := range []any{nil, 0, -1} {
		payload := map[string]any{
			"applicationId": "app-1",
			"revisionId":    uuid.NewString(),
			"replicaIndex":  0,
			"image":         "redis:7-alpine",
		}
		if number != nil {
			payload["revisionNumber"] = number
		}
		_, err := h.Execute(context.Background(), payload)
		execErr, ok := err.(*ExecutionError)
		if !ok || execErr.Code != ErrCodeInvalidPayload {
			t.Fatalf("revisionNumber %v: got %v", number, err)
		}
	}
	if called {
		t.Fatal("candidate creation ran without a valid revisionNumber")
	}
}

func TestDeployRevision_UsesRevisionNumberForIdentity(t *testing.T) {
	revisionID := uuid.NewString()
	prev := startDeployCandidate
	t.Cleanup(func() { startDeployCandidate = prev })
	var got candidate.CandidateSpec
	startDeployCandidate = func(_ context.Context, _ *docker.Client, _ *slog.Logger, spec candidate.CandidateSpec) (candidate.CandidateResult, error) {
		got = spec
		return candidate.CandidateResult{Status: "READY", ContainerName: "dc-redis-r1-1"}, nil
	}
	h := deployRevisionHandler(&docker.Client{}, runtimeTransport{}, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"applicationId":   "app-redis",
		"applicationSlug": "redis",
		"revisionId":      revisionID,
		"revisionNumber":  1,
		"replicaIndex":    0,
		"image":           "redis:7-alpine",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.RevisionID != revisionID {
		t.Fatalf("revisionId = %s", got.Metadata.RevisionID)
	}
	if got.Metadata.RevisionNumber != 1 || got.Metadata.Instance != 1 || got.Metadata.AppShortID != "redis" {
		t.Fatalf("metadata = %+v", got.Metadata)
	}
	token, err := appcontainer.NameRevisionToken(got.Metadata.RevisionNumber)
	if err != nil {
		t.Fatal(err)
	}
	name, err := appcontainer.FormatName(got.Metadata.AppShortID, token, got.Metadata.Instance)
	if err != nil {
		t.Fatal(err)
	}
	if name != "dc-redis-r1-1" {
		t.Fatalf("formatted name = %s", name)
	}
	platform, err := protocol.PlatformContainerName("redis", 1, 0)
	if err != nil || platform != name {
		t.Fatalf("platform name = %s err=%v", platform, err)
	}
}
