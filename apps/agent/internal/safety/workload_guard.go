package safety

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

var (
	ErrPrivilegedDenied        = errors.New("security violation: privileged containers are forbidden")
	ErrDockerSocketMountDenied = errors.New("security violation: mounting docker.sock into workload container is forbidden")
	ErrHostMountDenied         = errors.New("security violation: mounting host system root directory is forbidden")
	ErrHostNetworkDenied       = errors.New("security violation: host network or PID mode is forbidden")
	ErrCapabilityDenied        = errors.New("security violation: dangerous capability requested")
)

var forbiddenHostPaths = []string{
	"/",
	"/etc",
	"/proc",
	"/sys",
	"/dev",
	"/boot",
	"/root",
	"/usr",
	"/bin",
	"/sbin",
}

var forbiddenCapabilities = []string{
	"SYS_ADMIN",
	"CAP_SYS_ADMIN",
	"ALL",
	"CAP_SYS_PTRACE",
	"SYS_PTRACE",
	"CAP_SYS_RAWIO",
	"SYS_RAWIO",
	"CAP_SYS_MODULE",
	"SYS_MODULE",
}

// ValidateWorkloadSecurity verifies that container creation options do not violate security invariants.
func ValidateWorkloadSecurity(req docker.CreateContainerRequest) error {
	// 1. Privileged container rejection
	if req.Policy != nil && req.Policy.AllowPrivileged {
		return ErrPrivilegedDenied
	}

	// 2. Docker socket abuse rejection
	if req.Policy != nil && req.Policy.AllowDockerSocket {
		return ErrDockerSocketMountDenied
	}

	// 3. Host network or PID namespace rejection
	if req.Policy != nil && (req.Policy.AllowHostNetwork || req.Policy.AllowHostPID) {
		return ErrHostNetworkDenied
	}

	// 4. Dangerous capabilities rejection
	if req.Policy != nil {
		for _, capStr := range req.Policy.AddCapabilities {
			cleanCap := strings.ToUpper(strings.TrimSpace(capStr))
			for _, forbidden := range forbiddenCapabilities {
				if cleanCap == forbidden {
					return fmt.Errorf("%w: %s", ErrCapabilityDenied, capStr)
				}
			}
		}
	}

	// 5. Volume mounts & Host paths checks
	for _, vm := range req.Volumes {
		vName := filepath.Clean(vm.VolumeName)

		// Docker socket abuse check
		if strings.Contains(vName, "docker.sock") {
			return ErrDockerSocketMountDenied
		}

		// Root and system directories check
		for _, forbidden := range forbiddenHostPaths {
			if vName == forbidden || (forbidden != "/" && strings.HasPrefix(vName, forbidden+"/")) {
				return fmt.Errorf("%w: %s", ErrHostMountDenied, vName)
			}
		}
	}

	return nil
}
