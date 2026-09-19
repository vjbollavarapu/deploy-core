package metrics

import (
	"time"

	"github.com/google/uuid"
)

const (
	SourceAgent     = "agent"
	SourceHeartbeat = "heartbeat"
)

// ServerSnapshot is the current/summary view for a server.
type ServerSnapshot struct {
	ServerID         uuid.UUID
	OrganizationID   uuid.UUID
	RecordedAt       time.Time
	CPUPercent       *float64
	MemoryUsedBytes  *int64
	MemoryTotalBytes *int64
	DiskUsedBytes    *int64
	DiskTotalBytes   *int64
	Load1            *float64
	Load5            *float64
	Load15           *float64
	UptimeSeconds    *int64
	NetworkRxBytes   *int64
	NetworkTxBytes   *int64
	ContainerCount   *int
	Source           string
	Payload          map[string]any
	UpdatedAt        time.Time
}

// ContainerSnapshot is the current/summary view for one container.
type ContainerSnapshot struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	ServerID         uuid.UUID
	ApplicationID    *uuid.UUID
	ContainerID      string
	ContainerName    string
	RecordedAt       time.Time
	CPUPercent       *float64
	MemoryUsedBytes  *int64
	MemoryLimitBytes *int64
	NetworkRxBytes   *int64
	NetworkTxBytes   *int64
	RestartCount     *int
	Status           string
	Payload          map[string]any
	UpdatedAt        time.Time
}

type ServerIngest struct {
	CPUPercent       *float64
	MemoryUsedBytes  *int64
	MemoryTotalBytes *int64
	DiskUsedBytes    *int64
	DiskTotalBytes   *int64
	Load1            *float64
	Load5            *float64
	Load15           *float64
	UptimeSeconds    *int64
	NetworkRxBytes   *int64
	NetworkTxBytes   *int64
	ContainerCount   *int
	RecordedAt       *time.Time
	Payload          map[string]any
}

type ContainerIngest struct {
	ContainerID      string
	ContainerName    string
	ApplicationID    *uuid.UUID
	CPUPercent       *float64
	MemoryUsedBytes  *int64
	MemoryLimitBytes *int64
	NetworkRxBytes   *int64
	NetworkTxBytes   *int64
	RestartCount     *int
	Status           string
	RecordedAt       *time.Time
	Payload          map[string]any
}

type AgentIngestInput struct {
	Server     *ServerIngest
	Containers []ContainerIngest
}
