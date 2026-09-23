package protocol

import (
	"errors"
	"fmt"
	"strings"
)

// ContainerPrefix is the standard prefix for all platform-managed containers.
const ContainerPrefix = "dc"

// Platform Trusted Labels
const (
	LabelManaged         = "deploycore.managed"
	LabelApplicationID   = "deploycore.application_id"
	LabelRevisionID      = "deploycore.revision_id"
	LabelDeploymentID    = "deploycore.deployment_id"
	LabelEnvironmentID   = "deploycore.environment_id"
	LabelOrganizationID  = "deploycore.organization_id"
	LabelInstance        = "deploycore.instance"
	LabelApplicationSlug = "deploycore.application_slug"
	LabelCandidate       = "deploycore.candidate"
	LabelProtected       = "deploycore.protected"
)

// PortMapping specifies an isolated internal/external port configuration.
type PortMapping struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort,omitempty"`
	Protocol      string `json:"protocol,omitempty"` // tcp or udp, default tcp
}

// VolumeMount specifies a platform-managed volume attachment.
type VolumeMount struct {
	VolumeName string `json:"volumeName"`
	MountPath  string `json:"mountPath"`
	ReadOnly   bool   `json:"readOnly,omitempty"`
}

// HealthCheckConfig defines the health evaluation policy.
type HealthCheckConfig struct {
	Type               string   `json:"type"` // http, tcp, command
	Path               string   `json:"path,omitempty"`
	Port               int      `json:"port,omitempty"`
	Command            []string `json:"command,omitempty"`
	IntervalSeconds    int      `json:"intervalSeconds,omitempty"`
	TimeoutSeconds     int      `json:"timeoutSeconds,omitempty"`
	Retries            int      `json:"retries,omitempty"`
	StartPeriodSeconds int      `json:"startPeriodSeconds,omitempty"`
}

// RoutingConfig defines platform-owned reverse-proxy ingress rules.
type RoutingConfig struct {
	Domain      string `json:"domain"`
	PathPrefix  string `json:"pathPrefix,omitempty"`
	StripPrefix bool   `json:"stripPrefix,omitempty"`
	TargetPort  int    `json:"targetPort"`
	TLS         bool   `json:"tls,omitempty"`
}

// ContainerSpec defines a fully managed application container specification.
// Notice: Arbitrary host Docker configurations (HostConfig, Privileged, HostNetwork,
// HostPID, arbitrary host bind mounts) are strictly prohibited from this DTO.
type ContainerSpec struct {
	Name             string             `json:"name"`
	Image            string             `json:"image"`
	Command          []string           `json:"command,omitempty"`
	Entrypoint       []string           `json:"entrypoint,omitempty"`
	Environment      map[string]string  `json:"environment,omitempty"`
	Ports            []PortMapping      `json:"ports,omitempty"`
	CPULimit         float64            `json:"cpuLimit,omitempty"`
	MemoryLimitBytes int64              `json:"memoryLimitBytes,omitempty"`
	RestartPolicy    string             `json:"restartPolicy,omitempty"`
	ManagedNetworks  []string           `json:"managedNetworks,omitempty"`
	ManagedVolumes   []VolumeMount      `json:"managedVolumes,omitempty"`
	HealthCheck      *HealthCheckConfig `json:"healthCheck,omitempty"`
	Routing          *RoutingConfig     `json:"routing,omitempty"`
}

// Validate checks that the container specification is safe and compliant.
func (s *ContainerSpec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("container name is required")
	}
	if strings.TrimSpace(s.Image) == "" {
		return errors.New("container image is required")
	}

	// Validate volume mounts: no host path escapes
	for _, v := range s.ManagedVolumes {
		if strings.TrimSpace(v.VolumeName) == "" {
			return errors.New("volumeName cannot be empty")
		}
		if !strings.HasPrefix(v.MountPath, "/") {
			return fmt.Errorf("volume mountPath must be absolute: %s", v.MountPath)
		}
		if strings.Contains(v.MountPath, "..") {
			return fmt.Errorf("volume mountPath cannot contain path traversals: %s", v.MountPath)
		}
		// Disallow mounting sensitive system directories inside container
		cleanMount := strings.TrimRight(v.MountPath, "/")
		if cleanMount == "" || cleanMount == "/etc" || cleanMount == "/dev" ||
			cleanMount == "/proc" || cleanMount == "/sys" || cleanMount == "/var/run/docker.sock" {
			return fmt.Errorf("mounting to sensitive path %q is forbidden", v.MountPath)
		}
	}

	// Validate restart policy values
	if s.RestartPolicy != "" {
		switch s.RestartPolicy {
		case "no", "always", "on-failure", "unless-stopped":
			// Allowed
		default:
			return fmt.Errorf("invalid restart policy %q (allowed: no, always, on-failure, unless-stopped)", s.RestartPolicy)
		}
	}

	return nil
}
