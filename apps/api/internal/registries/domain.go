package registries

import (
	"time"

	"github.com/google/uuid"
)

const (
	ProviderGHCR      = "ghcr"
	ProviderDockerHub = "dockerhub"
	ProviderGCP       = "gcp"
	ProviderECR       = "ecr"
	ProviderACR       = "acr"
	ProviderOCI       = "oci"
)

const (
	StatusActive   = "active"
	StatusError    = "error"
	StatusDisabled = "disabled"
)

// Registry is an org-scoped container registry configuration.
// Credentials are encrypted at rest; API responses never include secrets.
type Registry struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Provider       string
	RegistryURL    string
	Username       string
	Status         string
	Metadata       map[string]any
	HasCredentials bool
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Credentials are used by agents for pull/push; never returned by the HTTP API.
type Credentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

type CreateInput struct {
	OrganizationID uuid.UUID
	Name           string
	Provider       string
	RegistryURL    string
	Username       string
	Credentials    *Credentials
	Metadata       map[string]any
}

type UpdateInput struct {
	Name           *string
	RegistryURL    *string
	Username       *string
	Status         *string
	Credentials    *Credentials
	ClearCredentials bool
	Metadata       map[string]any
}

type AuditMeta struct {
	IP        string
	UserAgent string
}
