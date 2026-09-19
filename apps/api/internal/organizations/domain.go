package organizations

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	Status    string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Role struct {
	ID          uuid.UUID
	Key         string
	Name        string
	Description string
	IsSystem    bool
}

type Member struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Email          string
	DisplayName    string
	Status         string
	Roles          []Role
	InvitedBy      *uuid.UUID
	JoinedAt       *time.Time
	CreatedAt      time.Time
}

type Invitation struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Email          string
	Roles          []Role
	InvitedBy      *uuid.UUID
	ExpiresAt      time.Time
	AcceptedAt     *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
	// RawToken is only set when creating an invitation (for delivery).
	RawToken string
}
