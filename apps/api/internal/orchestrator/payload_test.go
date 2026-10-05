package orchestrator

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

func TestContainerCreateCommandPayload_OmitsRuntimeValues(t *testing.T) {
	rev := uuid.New()
	port := 8000
	d := deployments.Deployment{
		ID:               uuid.New(),
		ApplicationID:    uuid.New(),
		TargetRevisionID: &rev,
		Trigger:          "manual",
	}
	const netName = "dc-modulyn-production-private"
	payload := containerCreateCommandPayload(d, 0, 1, "redis", "dc-redis-r1-1", nil, &port, netName, "modulyn", "production", nil)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"redis://redis:6379/1", "version-one-secret", "APP_SECRET"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("runtime value %q leaked into payload %s", secret, raw)
		}
	}
	if _, ok := payload["env"]; ok {
		t.Fatalf("payload includes env: %s", raw)
	}
	if payload["revisionId"] != rev.String() {
		t.Fatalf("revisionId = %v", payload["revisionId"])
	}
	ports, ok := payload["internalPorts"].([]map[string]any)
	if !ok || len(ports) != 1 {
		t.Fatalf("internalPorts = %#v", payload["internalPorts"])
	}
	if _, ok := ports[0]["hostPort"]; ok {
		t.Fatal("payload published a host port")
	}
	nets, ok := payload["networks"].([]string)
	if !ok || len(nets) != 1 || nets[0] != netName {
		t.Fatalf("networks = %#v", payload["networks"])
	}
	if payload["dnsAlias"] != "redis" {
		t.Fatalf("dnsAlias = %v", payload["dnsAlias"])
	}
}

func TestContainerCreateCommandPayload_RollbackUsesSamePrivateNetwork(t *testing.T) {
	rev := uuid.New()
	d := deployments.Deployment{
		ID:               uuid.New(),
		ApplicationID:    uuid.New(),
		TargetRevisionID: &rev,
		Trigger:          deployments.TriggerRollback,
	}
	const netName = "dc-modulyn-production-private"
	payload := containerCreateCommandPayload(d, 0, 1, "redis", "dc-redis-r1-1", nil, nil, netName, "modulyn", "production", nil)
	if payload["phase"] != "rollback" || payload["trigger"] != "rollback" {
		t.Fatalf("rollback markers = phase %v trigger %v", payload["phase"], payload["trigger"])
	}
	nets := payload["networks"].([]string)
	if nets[0] != netName || payload["dnsAlias"] != "redis" {
		t.Fatalf("rollback network payload = %#v", payload)
	}
	if _, ok := payload["hostPort"]; ok {
		t.Fatal("rollback published a host port")
	}
}

func TestCreatePrivateNetworkPayload_CanonicalAndIdempotentShape(t *testing.T) {
	name, err := protocol.FormatPrivateNetworkName("modulyn", "production")
	if err != nil {
		t.Fatal(err)
	}
	if name != "dc-modulyn-production-private" {
		t.Fatalf("network = %s", name)
	}
	first := createPrivateNetworkPayload("proj-1", "modulyn", "env-1", "production")
	second := createPrivateNetworkPayload("proj-1", "modulyn", "env-1", "production")
	raw1, _ := json.Marshal(first)
	raw2, _ := json.Marshal(second)
	if !bytes.Equal(raw1, raw2) {
		t.Fatalf("reissue payload changed: %s vs %s", raw1, raw2)
	}
	if first["networkType"] != protocol.NetworkTypePrivate || first["projectSlug"] != "modulyn" || first["environmentSlug"] != "production" {
		t.Fatalf("payload = %#v", first)
	}
	if _, ok := first["name"]; ok {
		t.Fatal("explicit name would bypass EnsurePrivateNetwork")
	}
}

func TestActivationProxyAttachedOnlyWithTraefik(t *testing.T) {
	plain := map[string]any{"phase": "enable_routing", "containerName": "dc-redis-r1-1"}
	attachProxyNetwork(plain, nil)
	if _, ok := plain["proxyNetwork"]; ok {
		t.Fatalf("no-domain activation attached proxy: %#v", plain)
	}
	routed := map[string]any{"phase": "enable_routing", "containerName": "dc-api-r1-1"}
	attachProxyNetwork(routed, map[string]any{"enabled": true})
	if routed["proxyNetwork"] != protocol.ProxyNetworkName {
		t.Fatalf("routed activation proxy = %#v", routed["proxyNetwork"])
	}
	if routed["traefik"] == nil {
		t.Fatal("routed activation dropped traefik config")
	}
}

func TestLocalBuildTagIsRevisionScoped(t *testing.T) {
	id := uuid.MustParse("742c2b91-80b8-4a8e-93ba-82ee7480d285")
	tag := localBuildTag(id)
	if tag != "deploycore-build:742c2b91-80b8-4a8e-93ba-82ee7480d285" || !isLocalBuildTag(tag) {
		t.Fatalf("tag=%q", tag)
	}
	for _, forbidden := range []string{"https://github.com/acme/app", "ghp_secret", "token", "://"} {
		if strings.Contains(tag, forbidden) {
			t.Fatalf("tag %q contains %q", tag, forbidden)
		}
	}
	if isLocalBuildTag("ghcr.io/example/api:1") || isLocalBuildTag("local:candidate") || isLocalBuildTag("deploycore-build:not-a-uuid") {
		t.Fatal("non-revision tag was treated as a local build")
	}
}

func TestRevisionRunImageKeepsRegistryPulls(t *testing.T) {
	image, policy := revisionRunImage("ghcr.io/example/api:1", "ghcr.io/example/api:1")
	if image != "ghcr.io/example/api:1" || policy != "" {
		t.Fatalf("image=%q policy=%q", image, policy)
	}
	image, policy = revisionRunImage("", "ghcr.io/example/api:1")
	if image != "ghcr.io/example/api:1" || policy != "" {
		t.Fatalf("fallback image=%q policy=%q", image, policy)
	}
	local := localBuildTag(uuid.MustParse("742c2b91-80b8-4a8e-93ba-82ee7480d285"))
	image, policy = revisionRunImage("sha256:abc", local)
	if image != local || policy != "never" {
		t.Fatalf("built image=%q policy=%q", image, policy)
	}
}

func TestAcceptBuiltImageRequiresIdentity(t *testing.T) {
	tag := localBuildTag(uuid.MustParse("742c2b91-80b8-4a8e-93ba-82ee7480d285"))
	if _, err := acceptBuiltImage(nil, tag); err == nil {
		t.Fatal("nil result accepted")
	}
	if _, err := acceptBuiltImage(map[string]any{"status": "built", "tags": []any{tag}}, tag); err == nil {
		t.Fatal("missing image id accepted")
	}
	if _, err := acceptBuiltImage(map[string]any{"imageId": "sha256:abc"}, tag); err == nil {
		t.Fatal("missing tag accepted")
	}
	id, err := acceptBuiltImage(map[string]any{"imageId": " sha256:abc ", "tags": []any{tag}}, tag)
	if err != nil || id != "sha256:abc" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestTargetHealthReplicasKeepsTargetRevisionOnly(t *testing.T) {
	target := uuid.New()
	previous := uuid.New()
	list := []replicas.Replica{
		{ReplicaIndex: 0, RevisionID: &target, ContainerName: "dc-api-r2-1"},
		{ReplicaIndex: 1, RevisionID: &previous, ContainerName: "dc-api-r1-2"},
		{ReplicaIndex: 2, RevisionID: &target, ContainerName: "dc-api-r2-3"},
	}
	got, err := targetHealthReplicas(list, &target, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ContainerName != "dc-api-r2-1" {
		t.Fatalf("replicas = %#v", got)
	}
	if _, err := targetHealthReplicas(list, &target, 2); err == nil {
		t.Fatal("previous-revision slot was accepted as a health target")
	}
}
