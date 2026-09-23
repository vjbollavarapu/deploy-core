package candidate

import (
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/appcontainer"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// PullPolicy defines when container images should be fetched from registries.
type PullPolicy string

const (
	PullIfNotPresent PullPolicy = "if_not_present"
	PullAlways       PullPolicy = "always"
	PullNever        PullPolicy = "never"
)

// HealthCheckType represents supported candidate health check mechanisms.
type HealthCheckType string

const (
	HealthTypeContainerState HealthCheckType = "CONTAINER_STATE"
	HealthTypeDocker         HealthCheckType = "DOCKER"
	HealthTypeHTTP           HealthCheckType = "HTTP"
	HealthTypeTCP            HealthCheckType = "TCP"
	HealthTypeCommand        HealthCheckType = "COMMAND"
)

// HealthPolicy defines candidate validation criteria before declaring readiness.
type HealthPolicy struct {
	Type             HealthCheckType `json:"type"`
	InitialDelay     time.Duration   `json:"initialDelay"`     // delay before starting checks
	Interval         time.Duration   `json:"interval"`         // time between probe attempts
	Timeout          time.Duration   `json:"timeout"`          // per-probe timeout
	FailureThreshold int             `json:"failureThreshold"` // consecutive failures before declaring failed
	SuccessThreshold int             `json:"successThreshold"` // consecutive successes required for readiness
	HTTPPath         string          `json:"httpPath,omitempty"`
	HTTPPort         int             `json:"httpPort,omitempty"`
	HTTPScheme       string          `json:"httpScheme,omitempty"`     // "http" or "https"
	ExpectedStatus   int             `json:"expectedStatus,omitempty"` // 0 = 200..399
	TCPPort          int             `json:"tcpPort,omitempty"`
	Command          []string        `json:"command,omitempty"`
}

// NetworkSpec specifies a required network for the candidate.
type NetworkSpec struct {
	Name     string
	Internal bool
}

// VolumeSpec specifies a required volume mount for the candidate.
type VolumeSpec struct {
	Name          string
	ContainerPath string
	ReadOnly      bool
}

// CandidateSpec represents the full deployment instruction to create and start a candidate revision.
type CandidateSpec struct {
	Metadata       appcontainer.Metadata
	Image          string
	PullPolicy     PullPolicy
	RegistryAuth   *docker.RegistryAuth
	Networks       []NetworkSpec
	Volumes        []VolumeSpec
	Entrypoint     []string
	Command        []string
	Env            []string
	InternalPorts  []docker.PortMapping
	CPUMillis      int64
	MemoryBytes    int64
	RestartPolicy  docker.RestartPolicy
	Traefik        *docker.TraefikConfig // preserved for activation; disabled during candidate start
	HealthPolicy   HealthPolicy
	StartupTimeout time.Duration
	ReadOnlyRootFS bool
	Labels         map[string]string
	Policy         *docker.PrivilegedPolicy
}

// CandidateResult represents the verified state of a candidate container ready for promotion.
type CandidateResult struct {
	Status         string            `json:"status"` // "READY"
	ContainerID    string            `json:"containerId"`
	ContainerName  string            `json:"containerName"`
	Image          string            `json:"image"`
	ImageID        string            `json:"imageId"`
	IPAddress      string            `json:"ipAddress"`
	NetworkIPs     map[string]string `json:"networkIPs"`
	HealthStatus   string            `json:"healthStatus"`
	StartedAt      time.Time         `json:"startedAt"`
	VerifiedLabels map[string]string `json:"verifiedLabels"`
	Observation    string            `json:"observation"`
	Duration       time.Duration     `json:"duration"`
}
