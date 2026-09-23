package rollback

import (
	"errors"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/candidate"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/drain"
)

var (
	// ErrRebuildForbidden indicates a rebuild request was received during a rollback.
	ErrRebuildForbidden = errors.New("agent rollback must not rebuild; target must use an existing immutable image")
	// ErrTargetHealthFailed indicates the candidate failed health checks; rollback was aborted.
	ErrTargetHealthFailed = errors.New("rollback target health check failed; traffic switch aborted and candidate cleaned")
	// ErrCandidateStartupFailed indicates the candidate container failed to start.
	ErrCandidateStartupFailed = errors.New("failed to start rollback candidate container")
	// ErrImageVerificationFailed indicates image was missing and could not be verified or pulled.
	ErrImageVerificationFailed = errors.New("rollback target image verification failed")
)

// RollbackSpec defines the parameters for rolling back to an immutable revision.
type RollbackSpec struct {
	OrganizationID       string                  `json:"organizationId"`
	ApplicationID        string                  `json:"applicationId"`
	DeploymentID         string                  `json:"deploymentId"`
	TargetRevisionID     string                  `json:"targetRevisionId"`
	ReplicaIndex         int                     `json:"replicaIndex"`
	Instance             int                     `json:"instance,omitempty"`
	ApplicationSlug      string                  `json:"applicationSlug,omitempty"`
	EnvironmentID        string                  `json:"environmentId,omitempty"`
	Image                string                  `json:"image"`
	ImageDigest          string                  `json:"imageDigest,omitempty"`
	Networks             []candidate.NetworkSpec `json:"networks,omitempty"`
	Volumes              []candidate.VolumeSpec  `json:"volumes,omitempty"`
	InternalPorts        []docker.PortMapping    `json:"internalPorts,omitempty"`
	CPUMillis            int64                   `json:"cpuMillis,omitempty"`
	MemoryBytes          int64                   `json:"memoryBytes,omitempty"`
	RestartPolicy        docker.RestartPolicy    `json:"restartPolicy,omitempty"`
	Env                  []string                `json:"env,omitempty"`
	Traefik              *docker.TraefikConfig   `json:"traefik,omitempty"`
	HealthPolicy         candidate.HealthPolicy  `json:"healthPolicy"`
	StartupTimeout       time.Duration           `json:"startupTimeout,omitempty"`
	ProxyNetwork         string                  `json:"proxyNetwork,omitempty"`
	CurrentContainerID   string                  `json:"currentContainerId,omitempty"`
	CurrentContainerName string                  `json:"currentContainerName,omitempty"`
	DrainDuration        time.Duration           `json:"drainDuration,omitempty"`
	TerminationTimeout   time.Duration           `json:"terminationTimeout,omitempty"`
	RetentionPolicy      drain.RetentionPolicy   `json:"retentionPolicy,omitempty"`
	// Rebuild rejection fields
	Dockerfile   string `json:"dockerfile,omitempty"`
	SourceRepo   string `json:"sourceRepo,omitempty"`
	BuildContext string `json:"buildContext,omitempty"`
}

// RollbackResult defines the structured outcome of a rollback execution.
type RollbackResult struct {
	Status                 string    `json:"status"` // "COMPLETED"
	TargetContainerID      string    `json:"targetContainerId"`
	TargetContainerName    string    `json:"targetContainerName"`
	Image                  string    `json:"image"`
	ImageDigest            string    `json:"imageDigest,omitempty"`
	HealthStatus           string    `json:"healthStatus"`
	CurrentContainerID     string    `json:"currentContainerId,omitempty"`
	CurrentContainerStatus string    `json:"currentContainerStatus,omitempty"`
	ActivatedAt            time.Time `json:"activatedAt"`
	DurationMs             int64     `json:"durationMs"`
	Summary                string    `json:"summary"`
}
