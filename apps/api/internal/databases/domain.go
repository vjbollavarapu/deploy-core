package databases

import (
	"time"

	"github.com/google/uuid"
)

const (
	EnginePostgreSQL = "postgresql"
)

const (
	StatusPending      = "PENDING"
	StatusProvisioning = "PROVISIONING"
	StatusRunning      = "RUNNING"
	StatusStopped      = "STOPPED"
	StatusDegraded     = "DEGRADED"
	StatusFailed       = "FAILED"
	StatusDeleting     = "DELETING"
	StatusDeleted      = "DELETED"
)

type Database struct {
	ID                  uuid.UUID
	OrganizationID      uuid.UUID
	ProjectID           uuid.UUID
	EnvironmentID       uuid.UUID
	ServerID            uuid.UUID
	Name                string
	Engine              string
	EngineVersion       string
	DatabaseName        string
	Username            string
	StorageVolumeName   string
	VolumeProtected     bool
	CPUMillis           *int
	MemoryBytes         *int64
	ContainerRuntimeID  *string
	Status              string
	BackupPolicy        map[string]any
	ProvisionCommandID  *uuid.UUID
	LastError           string
	CreatedBy           *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
	HasCredential       bool
}

type BackupPolicy struct {
	Enabled       bool   `json:"enabled"`
	Schedule      string `json:"schedule,omitempty"`
	RetentionDays int    `json:"retentionDays,omitempty"`
}

type CreateInput struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	ServerID       uuid.UUID
	Name           string
	Engine         string
	EngineVersion  string
	DatabaseName   string
	Username       string
	Password       string // optional; generated when empty
	StorageVolume  string // optional; default db-{name}-data
	CPUMillis      *int
	MemoryBytes    *int64
	BackupPolicy   map[string]any
}

type UpdateInput struct {
	Name           *string
	CPUMillis      *int
	MemoryBytes    *int64
	BackupPolicy   map[string]any
	Status         *string // limited operator transitions (STOPPED/RUNNING request via agent)
	RotatePassword *string
}

type AuditMeta struct {
	IP        string
	UserAgent string
}

type credentialBlob struct {
	Ciphertext []byte
	Nonce      []byte
	KeyID      string
}

type BootstrapSecrets struct {
	DatabaseID        uuid.UUID
	Engine            string
	EngineVersion     string
	DatabaseName      string
	Username          string
	Password          string
	StorageVolumeName string
	CPUMillis         *int
	MemoryBytes       *int64
}
