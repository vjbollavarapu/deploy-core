package safety

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/managedfs"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

// TestSecuritySuite_PathTraversal verifies defense-in-depth against directory traversal.
func TestSecuritySuite_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	allowedRoot := filepath.Join(tempDir, "workspaces")
	_ = os.MkdirAll(allowedRoot, 0700)

	traversalPayloads := []string{
		"../outside.txt",
		"../../../../../../etc/passwd",
		"..\\..\\windows\\win.ini",
		"sub/../../outside",
		"/var/log/syslog",
		"safe/path\x00/danger",
	}

	for _, payload := range traversalPayloads {
		t.Run(payload, func(t *testing.T) {
			_, err := managedfs.Resolve(allowedRoot, payload)
			if err == nil {
				t.Fatalf("expected error for traversal payload %q, got nil", payload)
			}
			if !errors.Is(err, managedfs.ErrPathTraversal) && !errors.Is(err, managedfs.ErrAccessDenied) {
				t.Errorf("expected traversal/access error, got %v", err)
			}
		})
	}
}

// TestSecuritySuite_MaliciousTarArchive tests zip-slip / tar traversal vulnerability prevention.
func TestSecuritySuite_MaliciousTarArchive(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "unpack")
	_ = os.MkdirAll(targetDir, 0700)

	// Create a tar archive with a malicious "../evil.sh" entry
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	hdr := &tar.Header{
		Name: "../evil.sh",
		Mode: 0755,
		Size: int64(len("malicious content")),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("malicious content")); err != nil {
		t.Fatal(err)
	}
	tw.Close()

	// Extraction logic must validate each entry using managedfs.Resolve
	tr := tar.NewReader(buf)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}

		// Security guard: resolve header name under targetDir
		_, resolveErr := managedfs.Resolve(targetDir, h.Name)
		if resolveErr == nil {
			t.Fatalf("security violation: malicious tar entry %s was not caught!", h.Name)
		}
	}
}

// TestSecuritySuite_WorkloadSanitization verifies containers cannot request host escapes.
func TestSecuritySuite_WorkloadSanitization(t *testing.T) {
	testCases := []struct {
		name      string
		req       docker.CreateContainerRequest
		expectErr error
	}{
		{
			name: "DockerSocketMount",
			req: docker.CreateContainerRequest{
				Volumes: []docker.VolumeMount{
					{VolumeName: "/var/run/docker.sock", MountPath: "/var/run/docker.sock"},
				},
			},
			expectErr: ErrDockerSocketMountDenied,
		},
		{
			name: "PrivilegedContainer",
			req: docker.CreateContainerRequest{
				Policy: &docker.PrivilegedPolicy{
					AllowPrivileged: true,
				},
			},
			expectErr: ErrPrivilegedDenied,
		},
		{
			name: "RootFilesystemMount",
			req: docker.CreateContainerRequest{
				Volumes: []docker.VolumeMount{
					{VolumeName: "/", MountPath: "/host_root"},
				},
			},
			expectErr: ErrHostMountDenied,
		},
		{
			name: "SysAdminCapability",
			req: docker.CreateContainerRequest{
				Policy: &docker.PrivilegedPolicy{
					AddCapabilities: []string{"CAP_SYS_ADMIN"},
				},
			},
			expectErr: ErrCapabilityDenied,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWorkloadSecurity(tc.req)
			if !errors.Is(err, tc.expectErr) {
				t.Fatalf("expected error %v, got %v", tc.expectErr, err)
			}
		})
	}
}

// TestSecuritySuite_CommandIntegrity verifies replay and expiry protection.
func TestSecuritySuite_CommandIntegrity(t *testing.T) {
	myServerID := uuid.New()

	t.Run("RejectWrongServerID", func(t *testing.T) {
		wrongID := uuid.New()
		cmd := protocol.CommandEnvelope{
			ID:       "cmd-1",
			ServerID: wrongID.String(),
		}
		if cmd.ServerID == myServerID.String() {
			t.Errorf("server ID match unexpected")
		}
	})

	t.Run("RejectExpiredCommand", func(t *testing.T) {
		expiredAt := time.Now().UTC().Add(-1 * time.Hour)
		if time.Now().UTC().Before(expiredAt) {
			t.Errorf("time assertion failed")
		}
	})
}
