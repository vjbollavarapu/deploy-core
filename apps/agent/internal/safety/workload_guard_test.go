package safety

import (
	"errors"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

func TestValidateWorkloadSecurity(t *testing.T) {
	t.Run("PermitSafeContainer", func(t *testing.T) {
		req := docker.CreateContainerRequest{
			Name: "dc-app-prod",
			Volumes: []docker.VolumeMount{
				{VolumeName: "dc-vol-data", MountPath: "/app/data"},
			},
		}
		if err := ValidateWorkloadSecurity(req); err != nil {
			t.Fatalf("unexpected error for safe container: %v", err)
		}
	})

	t.Run("RejectPrivileged", func(t *testing.T) {
		req := docker.CreateContainerRequest{
			Policy: &docker.PrivilegedPolicy{
				AllowPrivileged: true,
			},
		}
		if err := ValidateWorkloadSecurity(req); !errors.Is(err, ErrPrivilegedDenied) {
			t.Errorf("expected ErrPrivilegedDenied, got %v", err)
		}
	})

	t.Run("RejectDockerSocketMount", func(t *testing.T) {
		req := docker.CreateContainerRequest{
			Volumes: []docker.VolumeMount{
				{VolumeName: "/var/run/docker.sock", MountPath: "/var/run/docker.sock"},
			},
		}
		if err := ValidateWorkloadSecurity(req); !errors.Is(err, ErrDockerSocketMountDenied) {
			t.Errorf("expected ErrDockerSocketMountDenied, got %v", err)
		}
	})

	t.Run("RejectHostRootMount", func(t *testing.T) {
		req := docker.CreateContainerRequest{
			Volumes: []docker.VolumeMount{
				{VolumeName: "/", MountPath: "/host"},
			},
		}
		if err := ValidateWorkloadSecurity(req); !errors.Is(err, ErrHostMountDenied) {
			t.Errorf("expected ErrHostMountDenied, got %v", err)
		}
	})

	t.Run("RejectEtcMount", func(t *testing.T) {
		req := docker.CreateContainerRequest{
			Volumes: []docker.VolumeMount{
				{VolumeName: "/etc/shadow", MountPath: "/etc/shadow"},
			},
		}
		if err := ValidateWorkloadSecurity(req); !errors.Is(err, ErrHostMountDenied) {
			t.Errorf("expected ErrHostMountDenied, got %v", err)
		}
	})

	t.Run("RejectDangerousCapability", func(t *testing.T) {
		req := docker.CreateContainerRequest{
			Policy: &docker.PrivilegedPolicy{
				AddCapabilities: []string{"SYS_ADMIN"},
			},
		}
		if err := ValidateWorkloadSecurity(req); !errors.Is(err, ErrCapabilityDenied) {
			t.Errorf("expected ErrCapabilityDenied, got %v", err)
		}
	})

	t.Run("RejectHostNetworkMode", func(t *testing.T) {
		req := docker.CreateContainerRequest{
			Policy: &docker.PrivilegedPolicy{
				AllowHostNetwork: true,
			},
		}
		if err := ValidateWorkloadSecurity(req); !errors.Is(err, ErrHostNetworkDenied) {
			t.Errorf("expected ErrHostNetworkDenied, got %v", err)
		}
	})
}
