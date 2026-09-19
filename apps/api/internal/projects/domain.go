package projects

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Slug           string
	Description    string
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Environment struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	Name           string
	Slug           string
	Kind           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

const (
	KindProduction  = "production"
	KindStaging     = "staging"
	KindDevelopment = "development"
	KindPreview     = "preview"
	KindCustom      = "custom"
)
