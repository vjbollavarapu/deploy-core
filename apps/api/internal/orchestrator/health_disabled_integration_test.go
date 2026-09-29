package orchestrator

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestDisabledHealthCheckSkipsProbeAndActivates(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE application_configs
		SET image_reference = 'redis:7-alpine', internal_port = 6379,
		    health_check = '{"enabled": false}'::jsonb
		WHERE application_id = $1`, appID); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := deployments.NewPostgresRepository(pool)
	orch := New(pool, repo, log, Config{SimulateAgent: true})
	got := runDeploy(t, ctx, repo, orch, orgID, appID, envID, serverID, userID, "redis-1", deployments.TriggerManual, nil)
	if got.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", got.Status)
	}
	for _, cmd := range commandsFor(orch, got.ID) {
		if cmd.op == protocol.OpRunHealthCheck {
			t.Fatalf("disabled health check still issued a probe: %#v", cmd.payload)
		}
	}

	var healthRaw, effectiveRaw []byte
	if err := pool.QueryRow(ctx, `
		SELECT health_check, effective_config FROM revisions WHERE id = $1`, *got.ActiveRevisionID).
		Scan(&healthRaw, &effectiveRaw); err != nil {
		t.Fatal(err)
	}
	assertHealthDisabled(t, healthRaw)
	var effective map[string]any
	if err := json.Unmarshal(effectiveRaw, &effective); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(effective["healthCheck"])
	if err != nil {
		t.Fatal(err)
	}
	assertHealthDisabled(t, encoded)
}

func TestConfiguredHTTPHealthCheckStillProbes(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE application_configs
		SET health_check = '{"type":"HTTP","path":"/healthz","port":8080}'::jsonb
		WHERE application_id = $1`, appID); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := deployments.NewPostgresRepository(pool)
	orch := New(pool, repo, log, Config{SimulateAgent: true})
	got := runDeploy(t, ctx, repo, orch, orgID, appID, envID, serverID, userID, "http-1", deployments.TriggerManual, nil)
	if got.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", got.Status)
	}

	var probed bool
	for _, cmd := range commandsFor(orch, got.ID) {
		if cmd.op != protocol.OpRunHealthCheck {
			continue
		}
		probed = true
		if cmd.payload["path"] != "/healthz" || cmd.payload["type"] != "HTTP" {
			t.Fatalf("HTTP probe payload = %#v", cmd.payload)
		}
		if enabled, ok := cmd.payload["enabled"].(bool); !ok || !enabled {
			t.Fatalf("HTTP probe enabled = %#v", cmd.payload["enabled"])
		}
	}
	if !probed {
		t.Fatal("configured HTTP health check did not issue OpRunHealthCheck")
	}
}

func assertHealthDisabled(t *testing.T, raw []byte) {
	t.Helper()
	var health map[string]any
	if err := json.Unmarshal(raw, &health); err != nil {
		t.Fatal(err)
	}
	enabled, ok := health["enabled"].(bool)
	if !ok || enabled {
		t.Fatalf("health check = %#v, want enabled false", health)
	}
	if path, exists := health["path"]; exists && path != "" {
		t.Fatalf("disabled health check has path %#v", path)
	}
}
