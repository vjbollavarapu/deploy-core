package agents

import (
	"time"

	"github.com/google/uuid"
)

const (
	AgentStatusPending  = "pending"
	AgentStatusActive   = "active"
	AgentStatusRevoked  = "revoked"
	AgentStatusDisabled = "disabled"
)

type Agent struct {
	ID                    uuid.UUID
	OrganizationID        uuid.UUID
	ServerID              uuid.UUID
	AgentVersion          string
	Status                string
	RegistrationExpiresAt *time.Time
	RegistrationUsedAt    *time.Time
	RegistrationRevokedAt *time.Time
	LastSeenAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type RegistrationTokenResult struct {
	ServerID  uuid.UUID
	AgentID   uuid.UUID
	Token     string
	ExpiresAt time.Time
}

type RegisterResult struct {
	AgentID    uuid.UUID
	ServerID   uuid.UUID
	Credential string
}

type HeartbeatInput struct {
	Timestamp       *time.Time
	AgentVersion    string
	DockerStatus    string
	CPUPercent      *float64
	MemoryUsedBytes *int64
	DiskUsedBytes   *int64
	Load1           *float64
	ContainerCount  *int
	UptimeSeconds   *int64
	DockerVersion   *string

	Hostname         string
	OS               string
	Architecture     string
	CPUCores         int
	MemoryTotalBytes int64
	DiskTotalBytes   int64

	RunningContainers int
	ImageCount        int
	VolumeCount       int
	NetworkCount      int
	AgentState        string
}
