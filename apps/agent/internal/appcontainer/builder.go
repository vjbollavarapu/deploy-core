package appcontainer

import (
	"fmt"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// Spec represents the full specification for creating a platform application container.
type Spec struct {
	Metadata       Metadata
	Image          string
	Entrypoint     []string
	Command        []string
	Env            []string
	InternalPorts  []docker.PortMapping
	CPUMillis      int64
	MemoryBytes    int64
	RestartPolicy  docker.RestartPolicy
	Networks       []string
	Volumes        []docker.VolumeMount
	Traefik        *docker.TraefikConfig
	HealthCheck    *docker.HealthCheckConfig
	ReadOnlyRootFS bool
	Labels         map[string]string // user-supplied non-platform labels
	Policy         *docker.PrivilegedPolicy
}

// BuildCreateRequest transforms an application container specification into a validated
// docker.CreateContainerRequest equipped with platform naming and trusted labels.
func BuildCreateRequest(spec Spec) (docker.CreateContainerRequest, error) {
	if err := spec.Metadata.Validate(); err != nil {
		return docker.CreateContainerRequest{}, fmt.Errorf("invalid container metadata: %w", err)
	}

	name, err := FormatName(spec.Metadata.AppShortID, spec.Metadata.RevisionID, spec.Metadata.Instance)
	if err != nil {
		return docker.CreateContainerRequest{}, fmt.Errorf("could not format platform container name: %w", err)
	}

	trustedLabels := spec.Metadata.Labels()

	req := docker.CreateContainerRequest{
		Name:           name,
		Image:          spec.Image,
		Entrypoint:     spec.Entrypoint,
		Command:        spec.Command,
		Env:            spec.Env,
		InternalPorts:  spec.InternalPorts,
		CPUMillis:      spec.CPUMillis,
		MemoryBytes:    spec.MemoryBytes,
		RestartPolicy:  spec.RestartPolicy,
		Networks:       spec.Networks,
		Volumes:        spec.Volumes,
		Traefik:        spec.Traefik,
		HealthCheck:    spec.HealthCheck,
		ReadOnlyRootFS: spec.ReadOnlyRootFS,
		Labels:         spec.Labels,
		PlatformLabels: trustedLabels,
		Policy:         spec.Policy,
	}

	return req, nil
}
