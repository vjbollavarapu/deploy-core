package volumes

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatePending   = "PENDING"
	StateCreating  = "CREATING"
	StateReady     = "READY"
	StateAttached  = "ATTACHED"
	StateDetaching = "DETACHING"
	StateDeleting  = "DELETING"
	StateFailed    = "FAILED"
	StateDeleted   = "DELETED"
)

const (
	ResourceDatabase    = "database"
	ResourceApplication = "application"
	DriverLocal         = "local"
)

type Volume struct {
	ID                   uuid.UUID
	OrganizationID       uuid.UUID
	ServerID             uuid.UUID
	Name                 string
	Driver               string
	MountPath            string
	State                string
	AttachedResourceType *string
	AttachedResourceID   *uuid.UUID
	BackupPolicy         map[string]any
	Protected            bool
	DockerName           *string
	UsageBytes           *int64
	Labels               map[string]any
	LastCommandID        *uuid.UUID
	LastError            string
	CreatedBy            *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
}

type CreateInput struct {
	OrganizationID uuid.UUID
	ServerID       uuid.UUID
	Name           string
	Driver         string
	MountPath      string
	BackupPolicy   map[string]any
	Protected      bool
	Labels         map[string]any
}

type AttachInput struct {
	ResourceType string
	ResourceID   uuid.UUID
	MountPath    string
}

type UpdateInput struct {
	MountPath    *string
	BackupPolicy map[string]any
	Labels       map[string]any
}

type AuditMeta struct {
	IP        string
	UserAgent string
}
