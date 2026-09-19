package healthchecks_test

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/healthchecks"
)

func TestParsePolicyDefaultsAndTypes(t *testing.T) {
	p, err := healthchecks.ParsePolicy(nil)
	if err != nil || p.Type != healthchecks.TypeContainer || p.Retries != 3 {
		t.Fatalf("default=%#v err=%v", p, err)
	}

	p, err = healthchecks.ParsePolicy(map[string]any{
		"type": "http", "path": "/ready", "port": 8080, "retries": 2, "expectedStatus": 204,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != healthchecks.TypeHTTP || p.Path != "/ready" || p.ExpectedStatus != 204 || p.Retries != 2 {
		t.Fatalf("%#v", p)
	}

	if _, err := healthchecks.ParsePolicy(map[string]any{"type": "TCP"}); err == nil {
		t.Fatal("TCP requires port")
	}
	if _, err := healthchecks.ParsePolicy(map[string]any{"type": "COMMAND"}); err == nil {
		t.Fatal("COMMAND requires command")
	}
}

func TestNextStateAndActivationGate(t *testing.T) {
	if got := healthchecks.NextState(3, 0, 3, 3, healthchecks.StateStarting); got != healthchecks.StateHealthy {
		t.Fatalf("got %s", got)
	}
	if got := healthchecks.NextState(0, 3, 3, 3, healthchecks.StateStarting); got != healthchecks.StateUnhealthy {
		t.Fatalf("got %s", got)
	}
	if got := healthchecks.NextState(1, 1, 3, 3, healthchecks.StateHealthy); got != healthchecks.StateDegraded {
		t.Fatalf("got %s", got)
	}

	enabled := false
	p := healthchecks.Policy{Enabled: &enabled}
	if !healthchecks.ReadyForActivation(p, healthchecks.StateUnknown) {
		t.Fatal("disabled should activate")
	}
	en := true
	p.Enabled = &en
	if healthchecks.ReadyForActivation(p, healthchecks.StateStarting) {
		t.Fatal("starting should not activate")
	}
	if !healthchecks.ReadyForActivation(p, healthchecks.StateHealthy) {
		t.Fatal("healthy should activate")
	}
}
