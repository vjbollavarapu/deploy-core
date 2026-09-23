package docker_test

import (
	"encoding/json"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"testing"
)

func TestI4AgentTraefikFromControlPlaneShape(t *testing.T) {
	// Mimics domains.BuildAgentTraefikConfig output
	raw := []byte(`{
    "enabled": true,
    "serviceName": "i3-deploy-app",
    "port": 80,
    "domains": [
      {"hostname":"app.i4.test","isPrimary":true,"forceHTTPS":true,"certResolver":"letsencrypt"},
      {"hostname":"www.app.i4.test","isPrimary":false,"forceHTTPS":true,"certResolver":"letsencrypt","redirectToPrimary":true}
    ],
    "enableWebSocket": true,
    "traefikNetwork": "deploycore-proxy"
  }`)
	var tc docker.TraefikConfig
	if err := json.Unmarshal(raw, &tc); err != nil {
		t.Fatal(err)
	}
	labels := docker.GenerateTraefikLabels(&tc)
	if labels["traefik.enable"] != "true" {
		t.Fatal("enable")
	}
	if labels["traefik.docker.network"] != "deploycore-proxy" {
		t.Fatalf("network=%s", labels["traefik.docker.network"])
	}
	if labels["traefik.http.services.i3-deploy-app.loadbalancer.server.port"] != "80" {
		t.Fatal("port")
	}
	primaryRouter := "i3-deploy-app-app-i4-test"
	if labels["traefik.http.routers."+primaryRouter+".rule"] != "Host(`app.i4.test`)" {
		t.Fatalf("rule=%s", labels["traefik.http.routers."+primaryRouter+".rule"])
	}
	if labels["traefik.http.routers."+primaryRouter+".entrypoints"] != "websecure" {
		t.Fatal("entrypoints")
	}
	if labels["traefik.http.routers."+primaryRouter+".tls"] != "true" {
		t.Fatal("tls")
	}
	if labels["traefik.http.routers."+primaryRouter+".tls.certresolver"] != "letsencrypt" {
		t.Fatal("certresolver")
	}
	httpRouter := primaryRouter + "-http"
	redirMW := primaryRouter + "-https-redirect"
	if labels["traefik.http.routers."+httpRouter+".middlewares"] != redirMW {
		t.Fatalf("redirect mw=%s", labels["traefik.http.routers."+httpRouter+".middlewares"])
	}
	if labels["traefik.http.middlewares."+redirMW+".redirectscheme.scheme"] != "https" {
		t.Fatal("expected redirectscheme.scheme=https")
	}
	// Secondary redirect-to-primary intent
	found := false
	for k, v := range labels {
		if len(k) > 0 && (contains(k, "to-primary") || contains(k, "redirectregex")) {
			found = true
			_ = v
		}
	}
	if !found {
		t.Fatal("expected redirect-to-primary labels for secondary domain")
	}
}
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && indexOf(s, sub) >= 0))
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
