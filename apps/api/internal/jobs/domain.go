package jobs

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TypeDeploymentExecution  = "DEPLOYMENT_EXECUTION"
	TypeBackup               = "BACKUP"
	TypeRestore              = "RESTORE"
	TypeCertificateOperation = "CERTIFICATE_OPERATION"
	TypeNotificationDelivery = "NOTIFICATION_DELIVERY"
	TypeWebhookDelivery      = "WEBHOOK_DELIVERY"
	TypeReplicasReconcile    = "REPLICAS_RECONCILE"
	TypeDesiredStateReconcile = "DESIRED_STATE_RECONCILE"
)

const (
	StatusQueued    = "queued"
	StatusLeased    = "leased"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusDead      = "dead"
	StatusCancelled = "cancelled"
)

type Job struct {
	ID                  uuid.UUID
	OrganizationID      *uuid.UUID
	Type                string
	Status              string
	Payload             map[string]any
	IdempotencyKey      *string
	AttemptCount        int
	MaxAttempts         int
	AvailableAt         time.Time
	LeasedUntil         *time.Time
	LeaseOwner          *string
	LastError           *string
	RequestID           *string
	CorrelationID       *string
	RelatedResourceType *string
	RelatedResourceID   *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	FinishedAt          *time.Time
}

type EnqueueInput struct {
	OrganizationID      *uuid.UUID
	Type                string
	Payload             map[string]any
	IdempotencyKey      *string
	MaxAttempts         int
	AvailableAt         *time.Time
	RequestID           *string
	CorrelationID       *string
	RelatedResourceType *string
	RelatedResourceID   *uuid.UUID
}

func mustJSON(v any) []byte {
	if v == nil {
		v = map[string]any{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
