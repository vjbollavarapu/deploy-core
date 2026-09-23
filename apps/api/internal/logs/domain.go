package logs

import (
	"time"

	"github.com/google/uuid"
)

const (
	KindBuild            = "build"
	KindRuntime          = "runtime"
	KindDeploymentEvents = "deployment_events"
)

const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
	StreamSystem = "system"
)

// Entry is one log line or deployment event in the streaming contract.
type Entry struct {
	Cursor         string     `json:"cursor"`
	Timestamp      time.Time  `json:"timestamp"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	ApplicationID  *uuid.UUID `json:"applicationId,omitempty"`
	DeploymentID   *uuid.UUID `json:"deploymentId,omitempty"`
	RevisionID     *uuid.UUID `json:"revisionId,omitempty"`
	Kind           string     `json:"kind"`
	Stream         string     `json:"stream"` // stdout|stderr|system
	Message        string     `json:"message"`
	Sequence       uint64     `json:"sequence"`
}

type Query struct {
	OrganizationID uuid.UUID
	ApplicationID  *uuid.UUID
	DeploymentID   *uuid.UUID
	Kind           string
	Since          *time.Time
	Cursor         string
	Limit          int
	Follow         bool
}

type AppendInput struct {
	OrganizationID uuid.UUID
	ApplicationID  *uuid.UUID
	DeploymentID   *uuid.UUID
	RevisionID     *uuid.UUID
	Kind           string
	Stream         string
	Message        string
	Timestamp      time.Time
}

type AuditMeta struct {
	IP        string
	UserAgent string
}

// StreamKey identifies an in-memory (or remote) log buffer.
// Deployment ID is an entry filter, not part of the buffer key, so app-scoped
// queries can include lines that also reference a deployment.
func StreamKey(orgID uuid.UUID, kind string, applicationID *uuid.UUID) string {
	key := orgID.String() + "|" + kind
	if applicationID != nil {
		key += "|app:" + applicationID.String()
	}
	return key
}
