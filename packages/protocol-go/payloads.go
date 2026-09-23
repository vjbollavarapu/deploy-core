package protocol

import (
	"errors"
	"fmt"
	"strings"
)

// BuildImagePayload defines inputs for OpBuildImage.
type BuildImagePayload struct {
	ApplicationID   string            `json:"applicationId"`
	DeploymentID    string            `json:"deploymentId"`
	RevisionID      string            `json:"revisionId"`
	ImageTag        string            `json:"imageTag"`
	Dockerfile      string            `json:"dockerfile,omitempty"`
	ContextDir      string            `json:"contextDir,omitempty"`
	BuildContextURL string            `json:"buildContextUrl,omitempty"`
	BuildArgs       map[string]string `json:"buildArgs,omitempty"`
	NoCache         bool              `json:"noCache,omitempty"`
}

// Validate verifies BuildImagePayload.
func (p *BuildImagePayload) Validate() error {
	if strings.TrimSpace(p.ApplicationID) == "" {
		return errors.New("applicationId is required")
	}
	if strings.TrimSpace(p.ImageTag) == "" {
		return errors.New("imageTag is required")
	}
	return nil
}

// PullImagePayload defines inputs for OpPullImage.
type PullImagePayload struct {
	Image     string `json:"image"`
	AuthToken string `json:"authToken,omitempty"`
	Registry  string `json:"registry,omitempty"`
}

// Validate verifies PullImagePayload.
func (p *PullImagePayload) Validate() error {
	if strings.TrimSpace(p.Image) == "" {
		return errors.New("image is required")
	}
	return nil
}

// CreateContainerPayload defines inputs for OpCreateContainer.
type CreateContainerPayload struct {
	ApplicationID string        `json:"applicationId,omitempty"`
	RevisionID    string        `json:"revisionId,omitempty"`
	DeploymentID  string        `json:"deploymentId,omitempty"`
	Spec          ContainerSpec `json:"spec"`
}

// Validate verifies CreateContainerPayload.
func (p *CreateContainerPayload) Validate() error {
	return p.Spec.Validate()
}

// StartContainerPayload defines inputs for OpStartContainer.
type StartContainerPayload struct {
	ContainerID    string `json:"containerId,omitempty"`
	ContainerName  string `json:"containerName,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
}

// Validate verifies StartContainerPayload.
func (p *StartContainerPayload) Validate() error {
	if strings.TrimSpace(p.ContainerID) == "" && strings.TrimSpace(p.ContainerName) == "" {
		return errors.New("containerId or containerName is required")
	}
	return nil
}

// StopContainerPayload defines inputs for OpStopContainer.
type StopContainerPayload struct {
	ContainerID         string `json:"containerId,omitempty"`
	ContainerName       string `json:"containerName,omitempty"`
	DrainTimeoutSeconds int    `json:"drainTimeoutSeconds,omitempty"`
	GraceTimeoutSeconds int    `json:"graceTimeoutSeconds,omitempty"`
	ForceKill           bool   `json:"forceKill,omitempty"`
}

// Validate verifies StopContainerPayload.
func (p *StopContainerPayload) Validate() error {
	if strings.TrimSpace(p.ContainerID) == "" && strings.TrimSpace(p.ContainerName) == "" {
		return errors.New("containerId or containerName is required")
	}
	return nil
}

// RestartContainerPayload defines inputs for OpRestartContainer.
type RestartContainerPayload struct {
	ContainerID    string `json:"containerId,omitempty"`
	ContainerName  string `json:"containerName,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
}

// Validate verifies RestartContainerPayload.
func (p *RestartContainerPayload) Validate() error {
	if strings.TrimSpace(p.ContainerID) == "" && strings.TrimSpace(p.ContainerName) == "" {
		return errors.New("containerId or containerName is required")
	}
	return nil
}

// RemoveContainerPayload defines inputs for OpRemoveContainer.
type RemoveContainerPayload struct {
	ContainerID   string `json:"containerId,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	Force         bool   `json:"force,omitempty"`
	RemoveVolumes bool   `json:"removeVolumes,omitempty"`
}

// Validate verifies RemoveContainerPayload.
func (p *RemoveContainerPayload) Validate() error {
	if strings.TrimSpace(p.ContainerID) == "" && strings.TrimSpace(p.ContainerName) == "" {
		return errors.New("containerId or containerName is required")
	}
	return nil
}

// CreateNetworkPayload defines inputs for OpCreateNetwork.
type CreateNetworkPayload struct {
	NetworkName   string            `json:"networkName"`
	NetworkType   string            `json:"networkType,omitempty"`
	Driver        string            `json:"driver,omitempty"`
	ProjectID     string            `json:"projectId,omitempty"`
	EnvironmentID string            `json:"environmentId,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// Validate verifies CreateNetworkPayload.
func (p *CreateNetworkPayload) Validate() error {
	if strings.TrimSpace(p.NetworkName) == "" {
		return errors.New("networkName is required")
	}
	return nil
}

// RemoveNetworkPayload defines inputs for OpRemoveNetwork.
type RemoveNetworkPayload struct {
	NetworkID   string `json:"networkId,omitempty"`
	NetworkName string `json:"networkName,omitempty"`
}

// Validate verifies RemoveNetworkPayload.
func (p *RemoveNetworkPayload) Validate() error {
	if strings.TrimSpace(p.NetworkID) == "" && strings.TrimSpace(p.NetworkName) == "" {
		return errors.New("networkId or networkName is required")
	}
	return nil
}

// CreateVolumePayload defines inputs for OpCreateVolume.
type CreateVolumePayload struct {
	VolumeName    string            `json:"volumeName"`
	Driver        string            `json:"driver,omitempty"`
	ProjectID     string            `json:"projectId,omitempty"`
	EnvironmentID string            `json:"environmentId,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// Validate verifies CreateVolumePayload.
func (p *CreateVolumePayload) Validate() error {
	if strings.TrimSpace(p.VolumeName) == "" {
		return errors.New("volumeName is required")
	}
	return nil
}

// RemoveVolumePayload defines inputs for OpRemoveVolume.
type RemoveVolumePayload struct {
	VolumeID   string `json:"volumeId,omitempty"`
	VolumeName string `json:"volumeName,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

// Validate verifies RemoveVolumePayload.
func (p *RemoveVolumePayload) Validate() error {
	if strings.TrimSpace(p.VolumeID) == "" && strings.TrimSpace(p.VolumeName) == "" {
		return errors.New("volumeId or volumeName is required")
	}
	return nil
}

// RunHealthCheckPayload defines inputs for OpRunHealthCheck.
type RunHealthCheckPayload struct {
	ContainerID   string            `json:"containerId,omitempty"`
	ContainerName string            `json:"containerName,omitempty"`
	HealthCheck   HealthCheckConfig `json:"healthCheck"`
}

// Validate verifies RunHealthCheckPayload.
func (p *RunHealthCheckPayload) Validate() error {
	if strings.TrimSpace(p.ContainerID) == "" && strings.TrimSpace(p.ContainerName) == "" {
		return errors.New("containerId or containerName is required")
	}
	if strings.TrimSpace(p.HealthCheck.Type) == "" {
		return errors.New("healthCheck.type is required")
	}
	return nil
}

// ActivateRevisionPayload defines inputs for OpActivateRevision.
type ActivateRevisionPayload struct {
	ApplicationID string        `json:"applicationId"`
	RevisionID    string        `json:"revisionId"`
	ContainerID   string        `json:"containerId"`
	Routing       RoutingConfig `json:"routing"`
}

// Validate verifies ActivateRevisionPayload.
func (p *ActivateRevisionPayload) Validate() error {
	if strings.TrimSpace(p.ApplicationID) == "" {
		return errors.New("applicationId is required")
	}
	if strings.TrimSpace(p.ContainerID) == "" {
		return errors.New("containerId is required")
	}
	if strings.TrimSpace(p.Routing.Domain) == "" {
		return errors.New("routing.domain is required")
	}
	if p.Routing.TargetPort <= 0 {
		return fmt.Errorf("routing.targetPort must be > 0, got %d", p.Routing.TargetPort)
	}
	return nil
}

// DeactivateRevisionPayload defines inputs for OpDeactivateRevision.
type DeactivateRevisionPayload struct {
	ApplicationID       string `json:"applicationId"`
	RevisionID          string `json:"revisionId"`
	ContainerID         string `json:"containerId"`
	DrainTimeoutSeconds int    `json:"drainTimeoutSeconds,omitempty"`
}

// Validate verifies DeactivateRevisionPayload.
func (p *DeactivateRevisionPayload) Validate() error {
	if strings.TrimSpace(p.ContainerID) == "" {
		return errors.New("containerId is required")
	}
	return nil
}

// CreateDatabasePayload defines inputs for OpCreateDatabase.
type CreateDatabasePayload struct {
	DatabaseID       string  `json:"databaseId"`
	Engine           string  `json:"engine"`
	Version          string  `json:"version"`
	DatabaseName     string  `json:"databaseName"`
	Username         string  `json:"username"`
	Password         string  `json:"password"`
	MemoryLimitBytes int64   `json:"memoryLimitBytes,omitempty"`
	CPULimit         float64 `json:"cpuLimit,omitempty"`
	StorageVolume    string  `json:"storageVolume"`
	NetworkName      string  `json:"networkName"`
}

// Validate verifies CreateDatabasePayload.
func (p *CreateDatabasePayload) Validate() error {
	if strings.TrimSpace(p.DatabaseID) == "" {
		return errors.New("databaseId is required")
	}
	if strings.TrimSpace(p.DatabaseName) == "" {
		return errors.New("databaseName is required")
	}
	if strings.TrimSpace(p.Username) == "" {
		return errors.New("username is required")
	}
	if strings.TrimSpace(p.Password) == "" {
		return errors.New("password is required")
	}
	if strings.TrimSpace(p.StorageVolume) == "" {
		return errors.New("storageVolume is required")
	}
	return nil
}

// BackupDatabasePayload defines inputs for OpBackupDatabase.
type BackupDatabasePayload struct {
	DatabaseID      string `json:"databaseId"`
	BackupID        string `json:"backupId"`
	DestinationType string `json:"destinationType,omitempty"`
	DestinationPath string `json:"destinationPath,omitempty"`
	Compression     string `json:"compression,omitempty"`
}

// Validate verifies BackupDatabasePayload.
func (p *BackupDatabasePayload) Validate() error {
	if strings.TrimSpace(p.DatabaseID) == "" {
		return errors.New("databaseId is required")
	}
	if strings.TrimSpace(p.BackupID) == "" {
		return errors.New("backupId is required")
	}
	return nil
}

// RestoreDatabasePayload defines inputs for OpRestoreDatabase.
type RestoreDatabasePayload struct {
	DatabaseID       string `json:"databaseId"`
	BackupID         string `json:"backupId"`
	SourcePath       string `json:"sourcePath"`
	ExpectedChecksum string `json:"expectedChecksum,omitempty"`
}

// Validate verifies RestoreDatabasePayload.
func (p *RestoreDatabasePayload) Validate() error {
	if strings.TrimSpace(p.DatabaseID) == "" {
		return errors.New("databaseId is required")
	}
	if strings.TrimSpace(p.SourcePath) == "" {
		return errors.New("sourcePath is required")
	}
	return nil
}
