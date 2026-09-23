package replicas

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending   = "PENDING"
	StatusStarting  = "STARTING"
	StatusRunning   = "RUNNING"
	StatusUnhealthy = "UNHEALTHY"
	StatusStopping  = "STOPPING"
	StatusStopped   = "STOPPED"
	StatusFailed    = "FAILED"
)

const (
	MaxDesiredReplicas = 20
	DefaultDesired     = 1
)

// Replica is one observed instance slot for an application.
type Replica struct {
	ID                  uuid.UUID
	OrganizationID      uuid.UUID
	ApplicationID       uuid.UUID
	RevisionID          *uuid.UUID
	ServerID            *uuid.UUID
	ReplicaIndex        int
	ContainerName       string
	ContainerID         *string
	Status              string
	Healthy             bool
	RoutingEnabled      bool
	LastProbeAt         *time.Time
	LastError           string
	RestartAttemptCount int
	NextRestartAt       *time.Time
	LastReconcileAt     *time.Time
	ObservedExitCode    *int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Summary is desired vs observed for an application.
type Summary struct {
	ApplicationID    uuid.UUID
	DesiredReplicas  int
	ObservedReplicas int
	HealthyReplicas  int
	RoutingReplicas  int
	Replicas         []Replica
}

// ScaleInput changes desiredReplicas in runtime config and enqueues reconcile.
type ScaleInput struct {
	DesiredReplicas int
}

// ReportInput is agent-observed state for a replica slot.
type ReportInput struct {
	ReplicaIndex   int
	Status         string
	Healthy        *bool
	RoutingEnabled *bool
	ContainerID    *string
	ContainerName  *string
	LastError      *string
}

type AuditMeta struct {
	IP        string
	UserAgent string
}
