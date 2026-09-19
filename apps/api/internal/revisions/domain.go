package revisions

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusCreated  = "CREATED"
	StatusReady    = "READY"
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
	StatusFailed   = "FAILED"
	StatusArchived = "ARCHIVED"
)

// Revision is an immutable deployment snapshot.
type Revision struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	ApplicationID    uuid.UUID
	DeploymentID     *uuid.UUID
	RevisionNumber   int
	Status           string
	CommitSHA        *string
	ImageDigest      *string
	ImageTag         *string
	EffectiveConfig  map[string]any
	VariableSnapshot map[string]any
	SecretRefs       []any
	HealthCheck      map[string]any
	ResourceLimits   map[string]any
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
