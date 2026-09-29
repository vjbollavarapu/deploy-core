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

func TestParsePolicyDisabledHTTPDoesNotInventPath(t *testing.T) {
	disabled, err := healthchecks.ParsePolicy(map[string]any{"enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.IsEnabled() {
		t.Fatal("enabled=false must disable probing")
	}
	if disabled.Path != "" {
		t.Fatalf("disabled policy invented path %q", disabled.Path)
	}

	httpOff, err := healthchecks.ParsePolicy(map[string]any{"type": "HTTP", "enabled": false, "port": 6379})
	if err != nil {
		t.Fatal(err)
	}
	if httpOff.IsEnabled() || httpOff.Type != healthchecks.TypeHTTP || httpOff.Path != "" {
		t.Fatalf("disabled HTTP policy = %#v", httpOff)
	}
	if !healthchecks.ReadyForActivation(httpOff, healthchecks.StateUnknown) {
		t.Fatal("disabled HTTP health check must not block activation")
	}

	httpOn, err := healthchecks.ParsePolicy(map[string]any{
		"type": "HTTP", "path": "/healthz", "port": 8080,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !httpOn.IsEnabled() || httpOn.Path != "/healthz" || httpOn.Port == nil || *httpOn.Port != 8080 {
		t.Fatalf("configured HTTP policy changed: %#v", httpOn)
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
