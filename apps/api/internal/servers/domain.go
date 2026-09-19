package servers

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StatusOnline      = "ONLINE"
	StatusDegraded    = "DEGRADED"
	StatusOffline     = "OFFLINE"
	StatusMaintenance = "MAINTENANCE"
	StatusDisabled    = "DISABLED"
)

// Server is an organization-scoped compute host in the registry.
type Server struct {
	ID                   uuid.UUID
	OrganizationID       uuid.UUID
	Name                 string
	Provider             string
	Region               string
	Hostname             string
	PublicIP             *string
	PrivateIP            *string
	Architecture         string
	OperatingSystem      string
	CPUCores             *int
	MemoryBytes          *int64
	DiskBytes            *int64
	CPUAllocatedMillis   int
	MemoryAllocatedBytes int64
	DiskAllocatedBytes   int64
	DockerVersion        *string
	Status               string
	MaintenanceMode      bool
	LastHeartbeatAt      *time.Time
	Labels               map[string]string
	CreatedBy            *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CapacityView is inventory totals plus denormalized reservations.
type CapacityView struct {
	ServerID             uuid.UUID
	Name                 string
	Status               string
	MaintenanceMode      bool
	Labels               map[string]string
	CPUTotalMillis       int
	CPUAllocatedMillis   int
	CPUAvailableMillis   int
	MemoryTotalBytes     int64
	MemoryAllocatedBytes int64
	MemoryAvailableBytes int64
	DiskTotalBytes       int64
	DiskAllocatedBytes   int64
	DiskAvailableBytes   int64
}

// PlacementContext is the application snapshot needed for scheduling.
type PlacementContext struct {
	ApplicationID    uuid.UUID
	OrganizationID   uuid.UUID
	TargetServerID   *uuid.UUID
	PlacementPolicy  map[string]any
	CPULimitMillis   int
	MemoryLimitBytes int64
	DiskBytes        int64
}

// CreateInput is validated create payload (status is always OFFLINE initially).
type CreateInput struct {
	OrganizationID  uuid.UUID
	Name            string
	Provider        string
	Region          string
	Hostname        string
	PublicIP        *string
	PrivateIP       *string
	Architecture    string
	OperatingSystem string
	CPUCores        *int
	MemoryBytes     *int64
	DiskBytes       *int64
	DockerVersion   *string
	Labels          map[string]string
}

// UpdateInput holds optional mutable inventory fields. Status is not client-driven.
type UpdateInput struct {
	Name            *string
	Provider        *string
	Region          *string
	Hostname        *string
	PublicIP        *string
	PrivateIP       *string
	Architecture    *string
	OperatingSystem *string
	CPUCores        *int
	MemoryBytes     *int64
	DiskBytes       *int64
	DockerVersion   *string
	Labels          map[string]string
	ClearPublicIP   bool
	ClearPrivateIP  bool
	Disabled        *bool // when true → DISABLED; when false → leave maintenance/offline rules
}

func labelsOrEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
