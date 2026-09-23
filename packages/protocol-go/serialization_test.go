package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRegisterRequestSerialization(t *testing.T) {
	req := RegisterRequest{
		RegistrationToken: "tok_123",
		AgentVersion:      "v1.0.0",
		ProtocolMajor:     ProtocolMajor,
		ProtocolMinor:     ProtocolMinor,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled RegisterRequest
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatal(err)
	}
	if unmarshaled.RegistrationToken != "tok_123" || unmarshaled.ProtocolMajor != ProtocolMajor {
		t.Errorf("mismatched unmarshaled RegisterRequest: %+v", unmarshaled)
	}
}

func TestHeartbeatSerialization(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cpu := 12.5
	mem := int64(1024)
	req := HeartbeatRequest{
		Timestamp:       &now,
		AgentVersion:    "v1.0.0",
		ProtocolMajor:   ProtocolMajor,
		ProtocolMinor:   ProtocolMinor,
		DockerStatus:    "running",
		CPUPercent:      &cpu,
		MemoryUsedBytes: &mem,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled HeartbeatRequest
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatal(err)
	}
	if *unmarshaled.CPUPercent != 12.5 || unmarshaled.DockerStatus != "running" {
		t.Errorf("mismatched unmarshaled HeartbeatRequest: %+v", unmarshaled)
	}
}

func TestHostInventorySerialization(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	inv := HostInventory{
		Hostname:         "srv-prod-01",
		OS:               "linux",
		Architecture:     "amd64",
		CPUCores:         8,
		MemoryTotalBytes: 34359738368,
		DiskTotalBytes:   500107862016,
		DockerVersion:    "24.0.7",
		AgentVersion:     "v1.0.0",
		ProtocolMajor:    ProtocolMajor,
		ProtocolMinor:    ProtocolMinor,
		CollectedAt:      now,
	}
	b, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled HostInventory
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatal(err)
	}
	if unmarshaled.Hostname != "srv-prod-01" || unmarshaled.CPUCores != 8 {
		t.Errorf("mismatched unmarshaled HostInventory: %+v", unmarshaled)
	}
}

func TestCommandEnvelopeSerialization(t *testing.T) {
	cmd := CommandEnvelope{
		ProtocolVersion: ProtocolMajor,
		ID:              "cmd_123",
		CommandID:       "cmd_123",
		OrganizationID:  "org_123",
		ServerID:        "srv_123",
		Operation:       OpDeployRevision,
		SchemaVersion:   SchemaVersion,
		Status:          StatusPending,
		IssuedAt:        "2026-01-01T12:00:00Z",
		ExpiresAt:       "2026-01-01T13:00:00Z",
		CreatedAt:       "2026-01-01T12:00:00Z",
		UpdatedAt:       "2026-01-01T12:00:00Z",
	}
	b, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled CommandEnvelope
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatal(err)
	}
	if unmarshaled.GetCommandID() != "cmd_123" || unmarshaled.Operation != OpDeployRevision {
		t.Errorf("mismatched CommandEnvelope: %+v", unmarshaled)
	}
}

func TestContainerSpecSerialization(t *testing.T) {
	spec := ContainerSpec{
		Name:             "dc-web-r1-1",
		Image:            "ghcr.io/org/web:sha256-abc12345",
		Command:          []string{"npm", "start"},
		Environment:      map[string]string{"PORT": "3000"},
		CPULimit:         1.5,
		MemoryLimitBytes: 1073741824,
		RestartPolicy:    "unless-stopped",
		ManagedNetworks:  []string{"dc-net-prod"},
		ManagedVolumes: []VolumeMount{
			{VolumeName: "dc-vol-uploads", MountPath: "/app/uploads", ReadOnly: false},
		},
		HealthCheck: &HealthCheckConfig{
			Type:            "http",
			Path:            "/healthz",
			Port:            3000,
			IntervalSeconds: 10,
			TimeoutSeconds:  3,
			Retries:         3,
		},
		Routing: &RoutingConfig{
			Domain:     "example.com",
			PathPrefix: "/",
			TargetPort: 3000,
			TLS:        true,
		},
	}

	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled ContainerSpec
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatal(err)
	}
	if unmarshaled.Name != "dc-web-r1-1" || unmarshaled.Routing.Domain != "example.com" {
		t.Errorf("mismatched ContainerSpec: %+v", unmarshaled)
	}
}

func TestCommandAckAndCompletionSerialization(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ack := CommandAckRequest{
		CommandID:  "cmd_abc",
		ServerID:   "srv_123",
		AcceptedAt: now,
		WorkerID:   "worker-01",
		Status:     StatusAccepted,
	}
	b, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaledAck CommandAckRequest
	if err := json.Unmarshal(b, &unmarshaledAck); err != nil {
		t.Fatal(err)
	}
	if unmarshaledAck.CommandID != "cmd_abc" || unmarshaledAck.Status != StatusAccepted {
		t.Errorf("mismatched CommandAckRequest: %+v", unmarshaledAck)
	}

	comp := CommandCompletionRequest{
		CommandID:  "cmd_abc",
		Status:     StatusCompleted,
		Result:     map[string]any{"containerId": "c_999"},
		FinishedAt: now,
	}
	b2, err := json.Marshal(comp)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaledComp CommandCompletionRequest
	if err := json.Unmarshal(b2, &unmarshaledComp); err != nil {
		t.Fatal(err)
	}
	if unmarshaledComp.CommandID != "cmd_abc" || unmarshaledComp.Status != StatusCompleted {
		t.Errorf("mismatched CommandCompletionRequest: %+v", unmarshaledComp)
	}
}

func TestEventsSerialization(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	exitCode := 0
	event := RuntimeEvent{
		Type:          EventContainerStarted,
		ContainerID:   "c_123",
		ContainerName: "dc-app-r1-1",
		ApplicationID: "app-1",
		RevisionID:    "r1",
		Status:        "running",
		ExitCode:      &exitCode,
		Timestamp:     now,
	}
	b, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled RuntimeEvent
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatal(err)
	}
	if unmarshaled.Type != EventContainerStarted || unmarshaled.ContainerName != "dc-app-r1-1" {
		t.Errorf("mismatched RuntimeEvent: %+v", unmarshaled)
	}
}
