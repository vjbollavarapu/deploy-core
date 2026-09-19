package backups

import (
	"time"

	"github.com/google/uuid"
)

const (
	ResourceDatabase = "database"
	TypePGLogical    = "postgresql_logical"
)

const (
	DestLocal = "local"
	DestS3    = "s3" // reserved
)

const (
	StatusPending   = "PENDING"
	StatusQueued    = "QUEUED"
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
	StatusExpired   = "EXPIRED"
	StatusDeleted   = "DELETED"
)

const (
	RestorePending    = "PENDING"
	RestoreQueued     = "QUEUED"
	RestoreRunning    = "RUNNING"
	RestoreValidating = "VALIDATING"
	RestoreSucceeded  = "SUCCEEDED"
	RestoreFailed     = "FAILED"
	RestoreCancelled  = "CANCELLED"
)

// RestoreConfirmPhrase must be sent verbatim to authorize destructive restore.
const RestoreConfirmPhrase = "RESTORE"

type Backup struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	ServerID        uuid.UUID
	ResourceType    string
	ResourceID      uuid.UUID
	Type            string
	Status          string
	StartedAt       *time.Time
	CompletedAt     *time.Time
	DurationMs      *int64
	SizeBytes       *int64
	Checksum        string
	DestinationType string
	DestinationURI  string
	RetentionUntil  *time.Time
	JobID           *uuid.UUID
	CommandID       *uuid.UUID
	LastError       string
	Metadata        map[string]any
	CreatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type Restore struct {
	ID                 uuid.UUID
	OrganizationID     uuid.UUID
	BackupID           uuid.UUID
	TargetResourceType string
	TargetResourceID   uuid.UUID
	ServerID           uuid.UUID
	Status             string
	StartedAt          *time.Time
	CompletedAt        *time.Time
	DurationMs         *int64
	JobID              *uuid.UUID
	CommandID          *uuid.UUID
	ValidationPassed   *bool
	LastError          string
	Metadata           map[string]any
	CreatedBy          *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CreateBackupInput struct {
	DatabaseID     uuid.UUID
	Destination    string // local (default); s3 reserved
	RetentionDays  *int
	IdempotencyKey *string
}

type CreateRestoreInput struct {
	BackupID         uuid.UUID
	TargetDatabaseID uuid.UUID
	Confirm          string
	IdempotencyKey   *string
}

type AuditMeta struct {
	IP        string
	UserAgent string
}
