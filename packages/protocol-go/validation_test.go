package protocol

import (
	"strings"
	"testing"
	"time"
)

func TestCompatibilityRules(t *testing.T) {
	// Same major version is compatible
	if err := CheckCompatibility(ProtocolMajor, 0); err != nil {
		t.Fatalf("expected major %d to be compatible, got %v", ProtocolMajor, err)
	}
	if err := CheckCompatibility(ProtocolMajor, 5); err != nil {
		t.Fatalf("expected minor increment to be compatible, got %v", err)
	}

	// Different major version must be rejected
	if err := CheckCompatibility(ProtocolMajor+1, 0); err == nil {
		t.Fatalf("expected higher major version to be rejected")
	}
	if err := CheckCompatibility(ProtocolMajor-1, 0); err == nil {
		t.Fatalf("expected lower major version to be rejected")
	}

	if !IsCompatible(ProtocolMajor) {
		t.Errorf("expected IsCompatible(%d) to be true", ProtocolMajor)
	}
	if IsCompatible(ProtocolMajor + 1) {
		t.Errorf("expected IsCompatible(%d) to be false", ProtocolMajor+1)
	}
}

func TestCommandEnvelopeRejectsArbitraryShell(t *testing.T) {
	disallowedOps := []string{
		"EXEC_SHELL",
		"RUN_ARBITRARY_COMMAND",
		"exec_shell",
		"SHELL_EXECUTION",
	}

	for _, op := range disallowedOps {
		cmd := CommandEnvelope{
			ID:        "cmd-1",
			ServerID:  "srv-1",
			Operation: op,
			IssuedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		err := cmd.Validate()
		if err == nil {
			t.Errorf("expected operation %q to be rejected, but it passed", op)
		}
		if !strings.Contains(err.Error(), "arbitrary shell execution") {
			t.Errorf("expected shell execution error message, got %v", err)
		}
	}

	// Valid operation passes validation
	validCmd := CommandEnvelope{
		ID:        "cmd-1",
		ServerID:  "srv-1",
		Operation: OpStartContainer,
		IssuedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if err := validCmd.Validate(); err != nil {
		t.Errorf("expected valid command to pass, got %v", err)
	}
}

func TestContainerSpecValidation(t *testing.T) {
	// Missing name
	spec := ContainerSpec{Image: "alpine:latest"}
	if err := spec.Validate(); err == nil {
		t.Error("expected missing name to fail validation")
	}

	// Missing image
	spec = ContainerSpec{Name: "dc-app-r1-1"}
	if err := spec.Validate(); err == nil {
		t.Error("expected missing image to fail validation")
	}

	// Invalid restart policy
	spec = ContainerSpec{Name: "dc-app-r1-1", Image: "alpine:latest", RestartPolicy: "invalid-policy"}
	if err := spec.Validate(); err == nil {
		t.Error("expected invalid restart policy to fail validation")
	}

	// Dangerous mounts (mounting sensitive paths)
	forbiddenMounts := []string{
		"/",
		"/etc",
		"/proc",
		"/sys",
		"/dev",
		"/var/run/docker.sock",
		"/app/../etc",
	}

	for _, m := range forbiddenMounts {
		spec = ContainerSpec{
			Name:  "dc-app-r1-1",
			Image: "alpine:latest",
			ManagedVolumes: []VolumeMount{
				{VolumeName: "vol-1", MountPath: m},
			},
		}
		if err := spec.Validate(); err == nil {
			t.Errorf("expected mount path %q to fail validation", m)
		}
	}

	// Valid spec
	validSpec := ContainerSpec{
		Name:          "dc-app-r1-1",
		Image:         "alpine:latest",
		RestartPolicy: "unless-stopped",
		ManagedVolumes: []VolumeMount{
			{VolumeName: "vol-data", MountPath: "/data"},
		},
	}
	if err := validSpec.Validate(); err != nil {
		t.Errorf("expected valid spec to pass, got %v", err)
	}
}

func TestPayloadsValidation(t *testing.T) {
	// BuildImagePayload
	bip := BuildImagePayload{}
	if err := bip.Validate(); err == nil {
		t.Error("expected empty BuildImagePayload to fail")
	}
	bip = BuildImagePayload{ApplicationID: "app-1", ImageTag: "img:1"}
	if err := bip.Validate(); err != nil {
		t.Errorf("expected valid BuildImagePayload to pass, got %v", err)
	}

	// CreateDatabasePayload
	cdp := CreateDatabasePayload{}
	if err := cdp.Validate(); err == nil {
		t.Error("expected empty CreateDatabasePayload to fail")
	}
	cdp = CreateDatabasePayload{
		DatabaseID:    "db-1",
		DatabaseName:  "mydb",
		Username:      "user",
		Password:      "pass",
		StorageVolume: "dc-vol-db",
	}
	if err := cdp.Validate(); err != nil {
		t.Errorf("expected valid CreateDatabasePayload to pass, got %v", err)
	}
}

func TestRegistrationAndHeartbeatValidation(t *testing.T) {
	reg := RegisterRequest{}
	if err := reg.Validate(); err == nil {
		t.Error("expected empty RegisterRequest to fail")
	}
	reg = RegisterRequest{RegistrationToken: "tok", AgentVersion: "v1.0.0"}
	if err := reg.Validate(); err != nil {
		t.Errorf("expected valid RegisterRequest to pass, got %v", err)
	}

	hb := HeartbeatRequest{}
	if err := hb.Validate(); err == nil {
		t.Error("expected empty HeartbeatRequest to fail")
	}
	hb = HeartbeatRequest{AgentVersion: "v1.0.0", DockerStatus: "running"}
	if err := hb.Validate(); err != nil {
		t.Errorf("expected valid HeartbeatRequest to pass, got %v", err)
	}
}
