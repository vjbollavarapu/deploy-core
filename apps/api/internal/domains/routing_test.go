package domains_test

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/domains"
	"github.com/google/uuid"
)

func TestNormalizeHostname(t *testing.T) {
	h, err := domains.NormalizeHostname("  API.Example.COM. ")
	if err != nil || h != "api.example.com" {
		t.Fatalf("got %q err=%v", h, err)
	}
	if _, err := domains.NormalizeHostname("bad host"); err == nil {
		t.Fatal("expected invalid")
	}
}

func TestBuildRoutingConfigForceHTTPS(t *testing.T) {
	d := domains.Domain{
		ID:            uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ApplicationID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		EnvironmentID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Hostname:      "api.example.com",
		InternalPort:  8080,
		IsPrimary:     true,
		ForceHTTPS:    true,
	}
	cfg := domains.BuildRoutingConfig("my-api", d)
	if cfg.Provider != "traefik" {
		t.Fatalf("provider=%s", cfg.Provider)
	}
	if cfg.Labels["traefik.enable"] != "true" {
		t.Fatal("missing enable")
	}
	rule := cfg.Labels["traefik.http.routers.my-api-api-example-com.rule"]
	if rule != "Host(`api.example.com`)" {
		t.Fatalf("rule=%s", rule)
	}
	port := cfg.Labels["traefik.http.services.my-api.loadbalancer.server.port"]
	if port != "8080" {
		t.Fatalf("port=%s", port)
	}
	if cfg.Labels["deploycore.domain.primary"] != "true" {
		t.Fatal("missing primary marker")
	}
	if cfg.Labels["deploycore.routing.mode"] != "replicas" {
		t.Fatal("missing replicas routing mode")
	}
	if cfg.Labels["traefik.http.routers.my-api-api-example-com.tls"] != "true" {
		t.Fatal("missing tls")
	}
	if cfg.Labels["traefik.http.routers.my-api-api-example-com.tls.certresolver"] != "letsencrypt" {
		t.Fatal("missing certresolver")
	}
	if cfg.Labels["traefik.http.routers.my-api-api-example-com-http.entrypoints"] != "web" {
		t.Fatal("missing http redirect entrypoint")
	}
	rep := domains.BuildReplicaRoutingLabels("my-api", d, 2)
	if rep["deploycore.replica.index"] != "2" {
		t.Fatalf("replica index=%s", rep["deploycore.replica.index"])
	}
}

func TestBuildAgentTraefikConfigMultiDomain(t *testing.T) {
	primary := domains.Domain{
		Hostname:     "app.i4.test",
		InternalPort: 80,
		IsPrimary:    true,
		ForceHTTPS:   true,
	}
	secondary := domains.Domain{
		Hostname:     "www.app.i4.test",
		InternalPort: 80,
		IsPrimary:    false,
		ForceHTTPS:   true,
	}
	tc := domains.BuildAgentTraefikConfig("i4-deploy-app", []domains.Domain{primary, secondary})
	if tc == nil {
		t.Fatal("expected traefik payload")
	}
	if tc["enabled"] != true {
		t.Fatalf("enabled=%v", tc["enabled"])
	}
	if tc["serviceName"] != "i4-deploy-app" {
		t.Fatalf("serviceName=%v", tc["serviceName"])
	}
	if tc["port"] != 80 {
		t.Fatalf("port=%v", tc["port"])
	}
	if tc["traefikNetwork"] != "deploycore-proxy" {
		t.Fatalf("network=%v", tc["traefikNetwork"])
	}
	doms, ok := tc["domains"].([]map[string]any)
	if !ok || len(doms) != 2 {
		t.Fatalf("domains=%v", tc["domains"])
	}
	if doms[0]["certResolver"] != "letsencrypt" {
		t.Fatalf("certResolver=%v", doms[0]["certResolver"])
	}
	if doms[1]["redirectToPrimary"] != true {
		t.Fatalf("secondary redirect=%v", doms[1]["redirectToPrimary"])
	}
	if domains.BuildAgentTraefikConfig("x", nil) != nil {
		t.Fatal("empty domains should yield nil")
	}
}
