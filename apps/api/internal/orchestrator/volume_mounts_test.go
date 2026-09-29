package orchestrator

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/google/uuid"
)

func TestVolumeSnapshotPayloadAndCutover(t *testing.T) {
	raw := []any{
		map[string]any{"name": "redis-data", "mountPath": "/data", "readOnly": false},
		map[string]any{"name": "config", "mountPath": "/etc/app", "readOnly": true},
	}
	mounts := parseVolumeMounts(raw)
	if len(mounts) != 2 || !writableMounts(mounts) {
		t.Fatalf("mounts = %#v", mounts)
	}
	if err := validateWritableReplicas(true, 2); err == nil {
		t.Fatal("writable volume accepted more than one replica")
	}
	if err := validateWritableReplicas(false, 3); err != nil {
		t.Fatal(err)
	}
	if err := validateWritableReplicas(true, 1); err != nil {
		t.Fatal(err)
	}
	if cutoverStopsFirst(false) {
		t.Fatal("stateless deploy stopped the previous container first")
	}
	if !cutoverStopsFirst(true) {
		t.Fatal("writable deploy did not stop the previous container first")
	}
	readOnlyOnly := parseVolumeMounts([]any{
		map[string]any{"name": "config", "mountPath": "/etc/app", "readOnly": true},
	})
	if writableMounts(readOnlyOnly) || cutoverStopsFirst(writableMounts(readOnlyOnly)) {
		t.Fatal("read-only mounts changed rolling overlap")
	}

	prev := uuid.New()
	names := predecessorNames([]replicas.Replica{
		{RevisionID: &prev, ContainerName: "dc-redis-r1-1"},
		{ContainerName: "other"},
	}, prev)
	if len(names) != 1 || names[0] != "dc-redis-r1-1" {
		t.Fatalf("predecessors = %#v", names)
	}

	rev := uuid.New()
	d := deployments.Deployment{ID: uuid.New(), ApplicationID: uuid.New(), TargetRevisionID: &rev, Trigger: deployments.TriggerRollback}
	volumes := agentVolumePayload(mounts[:1])
	payload := containerCreateCommandPayload(d, 0, 1, "redis", "dc-redis-r2-1", nil, nil, "dc-modulyn-production-private", "modulyn", "production", volumes)
	got := payload["volumes"].([]map[string]any)
	if got[0]["name"] != "redis-data" || got[0]["containerPath"] != "/data" || got[0]["readOnly"] != false {
		t.Fatalf("rollback volumes = %#v", got)
	}
	if payload["phase"] != "rollback" {
		t.Fatalf("phase = %v", payload["phase"])
	}
	if payload["dnsAlias"] != "redis" {
		t.Fatal("volume payload dropped the private DNS alias")
	}
}

func TestReadOnlyLabel(t *testing.T) {
	if readOnlyLabel([]byte(`{"readOnly":true}`)) != true {
		t.Fatal("expected readOnly label")
	}
	if readOnlyLabel([]byte(`{}`)) {
		t.Fatal("missing label defaulted to read-only")
	}
}
