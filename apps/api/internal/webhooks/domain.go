package webhooks

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive   = "ACTIVE"
	StatusDisabled = "DISABLED"
	StatusFailing  = "FAILING"
)

const (
	EventDeploymentStarted   = "deployment.started"
	EventDeploymentCompleted = "deployment.completed"
	EventDeploymentFailed    = "deployment.failed"
	EventServerOffline       = "server.offline"
	EventBackupCompleted     = "backup.completed"
	EventBackupFailed        = "backup.failed"
)

const (
	DeliveryPending    = "PENDING"
	DeliveryQueued     = "QUEUED"
	DeliveryDelivering = "DELIVERING"
	DeliveryDelivered  = "DELIVERED"
	DeliveryFailed     = "FAILED"
	DeliverySkipped    = "SKIPPED"
)

func KnownEvents() []string {
	return []string{
		EventDeploymentStarted, EventDeploymentCompleted, EventDeploymentFailed,
		EventServerOffline, EventBackupCompleted, EventBackupFailed,
	}
}

type Webhook struct {
	ID                  uuid.UUID
	OrganizationID      uuid.UUID
	Name                string
	URL                 string
	Events              []string
	Enabled             bool
	Status              string
	ConsecutiveFailures int
	FailureThreshold    int
	LastError           string
	DisabledAt          *time.Time
	CreatedBy           *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
	// SecretPlain is returned once on create/rotate; never persisted in plaintext.
	SecretPlain *string
}

type Delivery struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	WebhookID      uuid.UUID
	EventType      string
	Payload        map[string]any
	Status         string
	AttemptCount   int
	JobID          *uuid.UUID
	ResponseCode   *int
	LatencyMs      *int
	LastError      string
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateInput struct {
	OrganizationID   uuid.UUID
	Name             string
	URL              string
	Events           []string
	Secret           string // optional; generated if empty
	Enabled          *bool
	FailureThreshold *int
}

type UpdateInput struct {
	Name             *string
	URL              *string
	Events           []string
	Enabled          *bool
	FailureThreshold *int
	RotateSecret     bool
}

type EmitInput struct {
	OrganizationID uuid.UUID
	EventType      string
	Payload        map[string]any
	ApplicationID  *uuid.UUID
	EnvironmentID  *uuid.UUID
	ServerID       *uuid.UUID
	ResourceType   string
	ResourceID     *uuid.UUID
}

type AuditMeta struct {
	IP        string
	UserAgent string
}

type secretBlob struct {
	Ciphertext []byte
	Nonce      []byte
	KeyID      string
}
