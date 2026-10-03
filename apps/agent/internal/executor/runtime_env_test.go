package executor

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/candidate"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/rollback"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

type runtimeTransport struct {
	boot protocol.RuntimeBootstrap
	err  error
}

func (runtimeTransport) Connect(context.Context) error           { return nil }
func (runtimeTransport) Disconnect(context.Context) error        { return nil }
func (runtimeTransport) Ping(context.Context) error              { return nil }
func (runtimeTransport) State() <-chan transport.ConnectionState { return nil }
func (runtimeTransport) CurrentState() transport.ConnectionState {
	return transport.StateConnected
}
func (runtimeTransport) PollCommands(context.Context) ([]protocol.CommandEnvelope, error) {
	return nil, nil
}
func (runtimeTransport) SendCommandStatus(context.Context, string, protocol.CommandStatusRequest) error {
	return nil
}
func (runtimeTransport) SendHeartbeat(context.Context, protocol.HeartbeatRequest) error { return nil }
func (runtimeTransport) SendLogs(context.Context, protocol.LogIngestRequest) error      { return nil }
func (runtimeTransport) SendMetrics(context.Context, protocol.MetricIngestRequest) error {
	return nil
}
func (runtimeTransport) FetchDatabaseBootstrap(context.Context, string) (protocol.DatabaseBootstrap, error) {
	return protocol.DatabaseBootstrap{}, nil
}
func (m runtimeTransport) FetchRevisionRuntime(context.Context, string) (protocol.RuntimeBootstrap, error) {
	return m.boot, m.err
}
func (runtimeTransport) FetchSourceAuth(context.Context, string) (protocol.SourceAuth, error) {
	return protocol.SourceAuth{Scheme: protocol.SourceAuthSchemeNone}, nil
}

func TestDeployRevision_InjectsBootstrapEnvAndHidesValues(t *testing.T) {
	rev := uuid.New()
	secret := "version-one-secret"
	prevStart := startDeployCandidate
	t.Cleanup(func() { startDeployCandidate = prevStart })

	var gotEnv []string
	startDeployCandidate = func(_ context.Context, _ *docker.Client, _ *slog.Logger, spec candidate.CandidateSpec) (candidate.CandidateResult, error) {
		gotEnv = append([]string(nil), spec.Env...)
		return candidate.CandidateResult{Status: "READY", ContainerID: "ctr-1", ContainerName: "dc-app-r1-1"}, nil
	}

	tr := runtimeTransport{boot: protocol.RuntimeBootstrap{
		RevisionID: rev.String(),
		Env: []protocol.RuntimeVariable{
			{Name: "REDIS_URL", Value: "redis://redis:6379/1"},
			{Name: "APP_SECRET", Value: secret},
		},
	}}
	h := deployRevisionHandler(&docker.Client{}, tr, nil)
	res, err := h.Execute(context.Background(), map[string]any{
		"applicationId":  "app-1",
		"revisionId":     rev.String(),
		"revisionNumber": 1,
		"image":          "redis:7-alpine",
		"env":            []string{"REDIS_URL=from-payload"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsEnv(gotEnv, "REDIS_URL=redis://redis:6379/1") || !containsEnv(gotEnv, "APP_SECRET="+secret) {
		t.Fatalf("container env = %#v", gotEnv)
	}
	raw, _ := json.Marshal(res.Output)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "redis://redis") {
		t.Fatalf("handler result leaked runtime values: %s", raw)
	}
}

func TestDeployRevision_SanitizesCreateError(t *testing.T) {
	rev := uuid.New()
	secret := "version-one-secret"
	prevStart := startDeployCandidate
	t.Cleanup(func() { startDeployCandidate = prevStart })
	startDeployCandidate = func(_ context.Context, _ *docker.Client, _ *slog.Logger, spec candidate.CandidateSpec) (candidate.CandidateResult, error) {
		return candidate.CandidateResult{}, errors.New("docker rejected " + spec.Env[0])
	}
	tr := runtimeTransport{boot: protocol.RuntimeBootstrap{
		RevisionID: rev.String(),
		Env:        []protocol.RuntimeVariable{{Name: "APP_SECRET", Value: secret}},
	}}
	h := deployRevisionHandler(&docker.Client{}, tr, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"applicationId":  "app-1",
		"revisionId":     rev.String(),
		"revisionNumber": 1,
		"image":          "example:1",
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked secret: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error = %v", err)
	}
}

func TestRollbackRevision_InjectsBootstrapEnv(t *testing.T) {
	rev := uuid.New()
	secret := "rolled-back-secret"
	prev := executeRollback
	t.Cleanup(func() { executeRollback = prev })
	var gotEnv []string
	executeRollback = func(_ context.Context, _ *docker.Client, _ *slog.Logger, spec rollback.RollbackSpec) (rollback.RollbackResult, error) {
		gotEnv = append([]string(nil), spec.Env...)
		return rollback.RollbackResult{Status: "COMPLETED", TargetContainerID: "ctr-old"}, nil
	}
	tr := runtimeTransport{boot: protocol.RuntimeBootstrap{
		RevisionID: rev.String(),
		Env:        []protocol.RuntimeVariable{{Name: "APP_SECRET", Value: secret}},
	}}
	h := rollbackRevisionHandler(&docker.Client{}, tr, nil)
	res, err := h.Execute(context.Background(), map[string]any{
		"applicationId":    "app-1",
		"targetRevisionId": rev.String(),
		"revisionNumber":   1,
		"image":            "example:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsEnv(gotEnv, "APP_SECRET="+secret) {
		t.Fatalf("rollback env = %#v", gotEnv)
	}
	raw, _ := json.Marshal(res.Output)
	if strings.Contains(string(raw), secret) {
		t.Fatalf("rollback result leaked secret: %s", raw)
	}
}

func TestDeployRevision_UUIDWithoutTransportFailsClosed(t *testing.T) {
	called := false
	prev := startDeployCandidate
	t.Cleanup(func() { startDeployCandidate = prev })
	startDeployCandidate = func(context.Context, *docker.Client, *slog.Logger, candidate.CandidateSpec) (candidate.CandidateResult, error) {
		called = true
		return candidate.CandidateResult{}, nil
	}
	h := deployRevisionHandler(&docker.Client{}, nil, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"applicationId":  "app-1",
		"revisionId":     uuid.NewString(),
		"revisionNumber": 1,
		"image":          "example:1",
	})
	if err == nil {
		t.Fatal("expected bootstrap failure")
	}
	if called {
		t.Fatal("container create ran without runtime bootstrap")
	}
	if !strings.Contains(err.Error(), "transport required") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeployRevision_PassesPrivateNetworkAndAlias(t *testing.T) {
	prev := startDeployCandidate
	t.Cleanup(func() { startDeployCandidate = prev })
	var got candidate.CandidateSpec
	startDeployCandidate = func(_ context.Context, _ *docker.Client, _ *slog.Logger, spec candidate.CandidateSpec) (candidate.CandidateResult, error) {
		got = spec
		return candidate.CandidateResult{Status: "READY", ContainerName: "dc-redis-r1-1"}, nil
	}
	h := deployRevisionHandler(&docker.Client{}, runtimeTransport{}, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"applicationId":   "app-1",
		"revisionId":      "r1",
		"revisionNumber":  1,
		"image":           "redis:7-alpine",
		"applicationSlug": "redis",
		"projectSlug":     "modulyn",
		"environmentSlug": "production",
		"networks":        []string{"dc-modulyn-production-private"},
		"dnsAlias":        "redis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.DNSAlias != "redis" || got.Metadata.ProjectSlug != "modulyn" {
		t.Fatalf("spec alias=%q project=%q", got.DNSAlias, got.Metadata.ProjectSlug)
	}
	if len(got.Networks) != 1 || got.Networks[0].Name != "dc-modulyn-production-private" {
		t.Fatalf("networks = %#v", got.Networks)
	}
}

func TestRollbackRevision_PassesPrivateNetworkAndAlias(t *testing.T) {
	prev := executeRollback
	t.Cleanup(func() { executeRollback = prev })
	var got rollback.RollbackSpec
	executeRollback = func(_ context.Context, _ *docker.Client, _ *slog.Logger, spec rollback.RollbackSpec) (rollback.RollbackResult, error) {
		got = spec
		return rollback.RollbackResult{Status: "COMPLETED"}, nil
	}
	h := rollbackRevisionHandler(&docker.Client{}, runtimeTransport{}, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"applicationId":    "app-1",
		"targetRevisionId": "rev-1",
		"revisionNumber":   1,
		"image":            "redis:7-alpine",
		"applicationSlug":  "redis",
		"projectSlug":      "modulyn",
		"environmentSlug":  "production",
		"networks":         []string{"dc-modulyn-production-private"},
		"dnsAlias":         "redis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.DNSAlias != "redis" || got.ProjectSlug != "modulyn" || got.EnvironmentSlug != "production" {
		t.Fatalf("rollback spec = %+v", got)
	}
	if len(got.Networks) != 1 || got.Networks[0].Name != "dc-modulyn-production-private" {
		t.Fatalf("rollback networks = %#v", got.Networks)
	}
	if got.ProxyNetwork != "" {
		t.Fatalf("rollback attached proxy %q", got.ProxyNetwork)
	}
}

func containsEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
