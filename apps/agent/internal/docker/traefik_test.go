package docker

import (
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestGenerateTraefikLabels_MultiDomainAndPrimary(t *testing.T) {
	cfg := &TraefikConfig{
		Enabled:         true,
		ServiceName:     "dayaapi",
		Port:            3000,
		TraefikNetwork:  protocol.ProxyNetworkName,
		EnableWebSocket: true,
		Domains: []DomainConfig{
			{
				Hostname:   "daya.io",
				IsPrimary:  true,
				ForceHTTPS: true,
				PathPrefix: "/api",
			},
			{
				Hostname:   "app.daya.io",
				IsPrimary:  false,
				ForceHTTPS: true,
			},
		},
	}

	labels := GenerateTraefikLabels(cfg)

	// 1. Explicit Network Control
	if labels["traefik.docker.network"] != protocol.ProxyNetworkName {
		t.Errorf("expected traefik.docker.network=%q, got %q", protocol.ProxyNetworkName, labels["traefik.docker.network"])
	}

	// 2. WebSocket compatibility
	if labels["traefik.http.services.dayaapi.loadbalancer.passhostheader"] != "true" {
		t.Errorf("expected passhostheader='true'")
	}
	if labels["traefik.http.services.dayaapi.loadbalancer.responseforwarding.flushinterval"] != "100ms" {
		t.Errorf("expected flushinterval='100ms'")
	}

	// 3. Primary domain router
	expectedPrimaryRule := "Host(`daya.io`) && PathPrefix(`/api`)"
	primaryRouter := "dayaapi-daya-io"
	if labels["traefik.http.routers."+primaryRouter+".rule"] != expectedPrimaryRule {
		t.Errorf("primary rule mismatch: got %q", labels["traefik.http.routers."+primaryRouter+".rule"])
	}
	if labels["traefik.http.routers."+primaryRouter+".entrypoints"] != "websecure" {
		t.Errorf("primary entrypoint mismatch")
	}
	if labels["deploycore.domain.primary"] != "true" {
		t.Errorf("expected deploycore.domain.primary='true'")
	}
	if labels["deploycore.domain.hostname"] != "daya.io" {
		t.Errorf("expected deploycore.domain.hostname='daya.io'")
	}

	// 4. Secondary domain router
	secondaryRouter := "dayaapi-app-daya-io"
	expectedSecondaryRule := "Host(`app.daya.io`)"
	if labels["traefik.http.routers."+secondaryRouter+".rule"] != expectedSecondaryRule {
		t.Errorf("secondary rule mismatch: got %q", labels["traefik.http.routers."+secondaryRouter+".rule"])
	}
}

func TestGenerateTraefikLabels_RedirectToPrimary(t *testing.T) {
	cfg := &TraefikConfig{
		Enabled:     true,
		ServiceName: "dayaapi",
		Port:        8080,
		Domains: []DomainConfig{
			{
				Hostname:   "daya.com",
				IsPrimary:  true,
				ForceHTTPS: true,
			},
			{
				Hostname:          "old-daya.com",
				IsPrimary:         false,
				RedirectToPrimary: true,
			},
		},
	}

	labels := GenerateTraefikLabels(cfg)

	// Verify redirect router and middleware on secondary domain
	redirRouter := "dayaapi-old-daya-com-to-primary"
	if labels["traefik.http.routers."+redirRouter+".rule"] != "Host(`old-daya.com`)" {
		t.Errorf("redirect router rule mismatch: %q", labels["traefik.http.routers."+redirRouter+".rule"])
	}

	redirMW := redirRouter + "-redir"
	if labels["traefik.http.middlewares."+redirMW+".redirectregex.replacement"] != "https://daya.com/${1}" {
		t.Errorf("unexpected redirect replacement: %q", labels["traefik.http.middlewares."+redirMW+".redirectregex.replacement"])
	}
	if labels["traefik.http.middlewares."+redirMW+".redirectregex.permanent"] != "true" {
		t.Errorf("expected permanent redirect='true'")
	}
}
