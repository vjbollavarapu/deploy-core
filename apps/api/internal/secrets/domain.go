package secrets

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

// Metadata is the API-safe secret view (never includes plaintext).
type Metadata struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Scope          string
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	Name           string
	Version        int
	KeyID          string
	Algorithm      string
	CreatedBy      *uuid.UUID
	UpdatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Record struct {
	Metadata
	Ciphertext []byte
	Nonce      []byte
}

type CreateInput struct {
	OrganizationID uuid.UUID
	Scope          string
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	Name           string
	Value          string
}

type UpdateInput struct {
	Value string
}
