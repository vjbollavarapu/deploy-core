package deployments

import (
	"time"

	"github.com/google/uuid"
)

const (
	TriggerManual   = "manual"
	TriggerGitPush  = "git_push"
	TriggerAPI      = "api"
	TriggerRollback = "rollback"
	TriggerSchedule = "schedule"
	TriggerSystem   = "system"
)

type Deployment struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	ApplicationID    uuid.UUID
	EnvironmentID    uuid.UUID
	ServerID         *uuid.UUID
	Status           string
	Trigger          string
	IdempotencyKey   *string
	RequestID        *string
	CorrelationID    *string
	CreatedBy        *uuid.UUID
	StartedAt        *time.Time
	FinishedAt       *time.Time
	ErrorCode        *string
	ErrorMessage     *string
	ActiveRevisionID *uuid.UUID
	TargetRevisionID *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Events           []Event
}

type Event struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	DeploymentID   uuid.UUID
	FromStatus     *string
	ToStatus       string
	Message        string
	Metadata       map[string]any
	RequestID      *string
	CreatedAt      time.Time
}

type CreateInput struct {
	ApplicationID  uuid.UUID
	Trigger        string
	IdempotencyKey *string
	CorrelationID  *string
	RequestID      string
}

type RollbackInput struct {
	ApplicationID    uuid.UUID
	TargetRevisionID uuid.UUID
	CorrelationID    *string
	RequestID        string
}

type TransitionInput struct {
	ToStatus     string
	Message      string
	Metadata     map[string]any
	RequestID    string
	ErrorCode    *string
	ErrorMessage *string
}
