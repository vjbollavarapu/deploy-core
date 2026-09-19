package variables

import (
	"time"

	"github.com/google/uuid"
)

const (
	ScopeOrganization = "ORGANIZATION"
	ScopeProject      = "PROJECT"
	ScopeEnvironment  = "ENVIRONMENT"
	ScopeApplication  = "APPLICATION"
)

type Variable struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Scope          string
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	Key            string
	Value          string
	CreatedBy      *uuid.UUID
	UpdatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateInput struct {
	OrganizationID uuid.UUID
	Scope          string
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	Key            string
	Value          string
}

type UpdateInput struct {
	Value *string
	Key   *string
}

// ResolvedEntry is a merged variable after inheritance.
type ResolvedEntry struct {
	Key            string
	Value          string
	Scope          string
	SourceID       uuid.UUID
	Overridden     bool
	OrganizationID uuid.UUID
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
}
