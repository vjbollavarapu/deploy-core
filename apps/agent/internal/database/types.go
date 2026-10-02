package database

import "time"

// ProvisionRequest specifies parameters for provisioning a stateful PostgreSQL database.
type ProvisionRequest struct {
	DatabaseID        string `json:"databaseId"`
	Engine            string `json:"engine"`
	EngineVersion     string `json:"engineVersion"`
	DatabaseName      string `json:"databaseName"`
	Username          string `json:"username"`
	Password          string `json:"password"` // Passed securely; never logged
	StorageVolumeName string `json:"storageVolumeName"`
	NetworkName       string `json:"networkName,omitempty"`
	DNSAlias          string `json:"dnsAlias,omitempty"`
	ProjectID         string `json:"projectId,omitempty"`
	ProjectSlug       string `json:"projectSlug,omitempty"`
	EnvironmentID     string `json:"environmentId,omitempty"`
	EnvironmentSlug   string `json:"environmentSlug,omitempty"`
	OrganizationID    string `json:"organizationId,omitempty"`
	CPUMillis         int64  `json:"cpuMillis,omitempty"`
	MemoryBytes       int64  `json:"memoryBytes,omitempty"`
	VolumeProtected   bool   `json:"volumeProtected"`
}

// DatabaseState represents the runtime state of a database container.
type DatabaseState struct {
	DatabaseID    string    `json:"databaseId"`
	ContainerID   string    `json:"containerId"`
	ContainerName string    `json:"containerName"`
	Status        string    `json:"status"`
	HealthStatus  string    `json:"healthStatus,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}
