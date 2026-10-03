package protocol

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

const (
	BuildPhaseFetchSource = "fetch_source"
	BuildPhaseBuild       = "build"
)

// BuildImagePayload is the BUILD_IMAGE contract shared by the control plane and the Agent.
// dockerfilePath is a path relative to contextPath. contextPath is relative to the deployment workspace.
// dockerfileContent is optional inline Dockerfile text, never a filesystem path.
type BuildImagePayload struct {
	Phase             string            `json:"phase"`
	DeploymentID      string            `json:"deploymentId"`
	ApplicationID     string            `json:"applicationId,omitempty"`
	RevisionID        string            `json:"revisionId,omitempty"`
	RepositoryURL     string            `json:"repositoryUrl,omitempty"`
	GitBranch         string            `json:"gitBranch,omitempty"`
	GitConnectionID   string            `json:"gitConnectionId,omitempty"`
	DockerfilePath    string            `json:"dockerfilePath,omitempty"`
	ContextPath       string            `json:"contextPath,omitempty"`
	DockerfileContent string            `json:"dockerfileContent,omitempty"`
	Archive           string            `json:"archive,omitempty"`
	ArchiveFormat     string            `json:"archiveFormat,omitempty"`
	StripComponents   int               `json:"stripComponents,omitempty"`
	Files             map[string]string `json:"files,omitempty"`
	RetentionPolicy   string            `json:"retentionPolicy,omitempty"`
	BuildArgs         map[string]string `json:"buildArgs,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	Target            string            `json:"target,omitempty"`
	TargetStage       string            `json:"targetStage,omitempty"`
	Platform          string            `json:"platform,omitempty"`
	CachePolicy       string            `json:"cachePolicy,omitempty"`
	TimeoutSeconds    *int              `json:"timeoutSeconds,omitempty"`
}

// Validate checks the phase and the source or build fields that phase requires.
func (p *BuildImagePayload) Validate() error {
	if strings.TrimSpace(p.DeploymentID) == "" {
		return errors.New("deploymentId is required")
	}
	if strings.ContainsAny(p.DeploymentID, "/\\") || strings.Contains(p.DeploymentID, "..") {
		return errors.New("deploymentId is invalid")
	}
	switch strings.TrimSpace(p.Phase) {
	case BuildPhaseFetchSource:
		if strings.TrimSpace(p.RepositoryURL) == "" && strings.TrimSpace(p.Archive) == "" && len(p.Files) == 0 {
			return errors.New("repositoryUrl is required to fetch source")
		}
		if strings.TrimSpace(p.RepositoryURL) != "" || strings.TrimSpace(p.GitBranch) != "" {
			if err := validateRepositoryURL(p.RepositoryURL); err != nil {
				return err
			}
			if err := validateGitBranch(p.GitBranch); err != nil {
				return err
			}
		}
		return validateGitConnectionID(p.GitConnectionID)
	case BuildPhaseBuild:
		if strings.TrimSpace(p.GitConnectionID) != "" {
			return errors.New("gitConnectionId is not allowed on build")
		}
		if err := validateRelativeBuildPath("contextPath", p.ContextPath); err != nil {
			return err
		}
		if err := validateRelativeBuildPath("dockerfilePath", p.DockerfilePath); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("phase must be fetch_source or build")
	}
}

func validateRepositoryURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \r\n\t\x00") {
		return errors.New("repositoryUrl must be an https URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil {
		return errors.New("repositoryUrl must be an https URL without credentials")
	}
	return nil
}

func validateGitConnectionID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	parts := strings.Split(id, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return errors.New("gitConnectionId is invalid")
	}
	for _, part := range parts {
		for _, c := range part {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return errors.New("gitConnectionId is invalid")
			}
		}
	}
	return nil
}

func validateGitBranch(branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.ContainsAny(branch, " \t\r\n\x00") || strings.Contains(branch, "..") || strings.HasPrefix(branch, "-") {
		return errors.New("gitBranch is invalid")
	}
	return nil
}

func validateRelativeBuildPath(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	invalid := strings.ContainsAny(value, "\r\n\x00") ||
		strings.HasPrefix(value, "/") ||
		strings.Contains(value, `\`)
	cleaned := path.Clean(strings.ReplaceAll(value, `\`, "/"))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		invalid = true
	}
	if !invalid {
		return nil
	}
	switch field {
	case "contextPath":
		return errors.New("invalid build context")
	case "dockerfilePath":
		return errors.New("invalid dockerfile path")
	default:
		return fmt.Errorf("invalid %s", field)
	}
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
