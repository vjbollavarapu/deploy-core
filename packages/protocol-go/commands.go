package protocol

import (
	"errors"
	"strings"
	"time"
)

// SchemaVersion is the current command contract version.
const SchemaVersion = 1

// Agent Command Operations
const (
	OpBuildImage         = "BUILD_IMAGE"
	OpPullImage          = "PULL_IMAGE"
	OpCreateContainer    = "CREATE_CONTAINER"
	OpStartContainer     = "START_CONTAINER"
	OpStopContainer      = "STOP_CONTAINER"
	OpRestartContainer   = "RESTART_CONTAINER"
	OpRemoveContainer    = "REMOVE_CONTAINER"
	OpCreateNetwork      = "CREATE_NETWORK"
	OpRemoveNetwork      = "REMOVE_NETWORK"
	OpInspectNetwork     = "INSPECT_NETWORK"
	OpCreateVolume       = "CREATE_VOLUME"
	OpRemoveVolume       = "REMOVE_VOLUME"
	OpAttachVolume       = "ATTACH_VOLUME"
	OpDetachVolume       = "DETACH_VOLUME"
	OpInspectVolume      = "INSPECT_VOLUME"
	OpRunHealthCheck     = "RUN_HEALTH_CHECK"
	OpActivateRevision   = "ACTIVATE_REVISION"
	OpDeactivateRevision = "DEACTIVATE_REVISION"
	OpDeployRevision     = "DEPLOY_REVISION"
	OpRollbackRevision   = "ROLLBACK_REVISION"
	OpCreateDatabase     = "CREATE_DATABASE"
	OpBackupDatabase     = "BACKUP_DATABASE"
	OpRestoreDatabase    = "RESTORE_DATABASE"
	OpProvisionDatabase  = "PROVISION_DATABASE"
	OpStartDatabase      = "START_DATABASE"
	OpStopDatabase       = "STOP_DATABASE"
	OpCreateBackup       = "CREATE_BACKUP"
	OpRestoreBackup      = "RESTORE_BACKUP"
	OpCollectStats       = "COLLECT_STATS"
	OpFetchLogs          = "FETCH_LOGS"
	OpStreamLogs         = "STREAM_LOGS"
	OpReconcile          = "RECONCILE"
	OpCleanupDisk        = "CLEANUP_DISK"
	OpCancelCommand      = "CANCEL_COMMAND"
)

// Standard Stable Agent Error Codes
const (
	ErrInvalidCommand         = "INVALID_COMMAND"
	ErrCommandExpired         = "COMMAND_EXPIRED"
	ErrCommandReplayed        = "COMMAND_REPLAYED"
	ErrUnauthorizedCommand    = "UNAUTHORIZED_COMMAND"
	ErrDockerUnavailable      = "DOCKER_UNAVAILABLE"
	ErrImageNotFound          = "IMAGE_NOT_FOUND"
	ErrImagePullFailed        = "IMAGE_PULL_FAILED"
	ErrImageBuildFailed       = "IMAGE_BUILD_FAILED"
	ErrContainerNotFound      = "CONTAINER_NOT_FOUND"
	ErrContainerCreateFailed  = "CONTAINER_CREATE_FAILED"
	ErrContainerStartFailed   = "CONTAINER_START_FAILED"
	ErrContainerStopFailed    = "CONTAINER_STOP_FAILED"
	ErrNetworkNotFound        = "NETWORK_NOT_FOUND"
	ErrNetworkConflict        = "NETWORK_CONFLICT"
	ErrVolumeNotFound         = "VOLUME_NOT_FOUND"
	ErrVolumeInUse            = "VOLUME_IN_USE"
	ErrHealthCheckFailed      = "HEALTH_CHECK_FAILED"
	ErrInsufficientDisk       = "INSUFFICIENT_DISK"
	ErrInsufficientMemory     = "INSUFFICIENT_MEMORY"
	ErrBackupFailed           = "BACKUP_FAILED"
	ErrRestoreFailed          = "RESTORE_FAILED"
	ErrRoutingFailed          = "ROUTING_FAILED"
	ErrWorkspaceInvalid       = "WORKSPACE_INVALID"
	ErrFilesystemAccessDenied = "FILESYSTEM_ACCESS_DENIED"
	ErrDisallowedOperation    = "DISALLOWED_OPERATION"
)

// Agent Command Statuses
const (
	StatusPending   = "pending"
	StatusAccepted  = "accepted"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"
)

// CommandEnvelope represents the structured instruction sent to an agent.
type CommandEnvelope struct {
	// ProtocolVersion indicates the wire version of this envelope.
	ProtocolVersion int `json:"protocolVersion,omitempty"`

	// ID is the primary identifier of the command (alias CommandID).
	ID string `json:"id"`

	// CommandID is the explicit camelCase identifier field for compatibility.
	CommandID string `json:"commandId,omitempty"`

	OrganizationID string         `json:"organizationId"`
	ServerID       string         `json:"serverId"`
	Operation      string         `json:"operation"`
	SchemaVersion  int            `json:"schemaVersion"`
	Payload        map[string]any `json:"payload"`
	Status         string         `json:"status"`
	IssuedAt       string         `json:"issuedAt"`
	ExpiresAt      string         `json:"expiresAt"`
	RequestID      *string        `json:"requestId,omitempty"`
	CorrelationID  *string        `json:"correlationId,omitempty"`
	IssuedBy       *string        `json:"issuedBy,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
	ErrorCode      *string        `json:"errorCode,omitempty"`
	ErrorMessage   *string        `json:"errorMessage,omitempty"`
	AcceptedAt     *string        `json:"acceptedAt,omitempty"`
	StartedAt      *string        `json:"startedAt,omitempty"`
	FinishedAt     *string        `json:"finishedAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

// GetCommandID returns the effective command identifier.
func (c *CommandEnvelope) GetCommandID() string {
	if c.CommandID != "" {
		return c.CommandID
	}
	return c.ID
}

// Validate verifies envelope integrity and prevents dangerous operations.
func (c *CommandEnvelope) Validate() error {
	id := c.GetCommandID()
	if strings.TrimSpace(id) == "" {
		return errors.New("commandId is required")
	}
	if strings.TrimSpace(c.ServerID) == "" {
		return errors.New("serverId is required")
	}
	if strings.TrimSpace(c.Operation) == "" {
		return errors.New("operation is required")
	}

	// Explicit security check: Arbitrary shell execution commands are strictly prohibited
	upperOp := strings.ToUpper(strings.TrimSpace(c.Operation))
	if upperOp == "EXEC_SHELL" || upperOp == "RUN_ARBITRARY_COMMAND" || strings.Contains(upperOp, "SHELL") {
		return errors.New("disallowed operation: arbitrary shell execution is strictly prohibited")
	}

	if strings.TrimSpace(c.IssuedAt) == "" {
		return errors.New("issuedAt timestamp is required")
	}
	return nil
}

// CommandAckRequest is sent by an agent to acknowledge receipt of a command.
type CommandAckRequest struct {
	CommandID  string    `json:"commandId"`
	ServerID   string    `json:"serverId"`
	AcceptedAt time.Time `json:"acceptedAt"`
	WorkerID   string    `json:"workerId,omitempty"`
	Status     string    `json:"status"`
}

// Validate checks that required ack fields are populated.
func (a *CommandAckRequest) Validate() error {
	if strings.TrimSpace(a.CommandID) == "" {
		return errors.New("commandId is required in ack")
	}
	if strings.TrimSpace(a.ServerID) == "" {
		return errors.New("serverId is required in ack")
	}
	return nil
}

// CommandProgressRequest reports intermediate execution progress to the control plane.
type CommandProgressRequest struct {
	CommandID       string    `json:"commandId"`
	Stage           string    `json:"stage"`
	ProgressPercent float64   `json:"progressPercent"`
	Message         string    `json:"message"`
	Timestamp       time.Time `json:"timestamp"`
}

// Validate checks progress report integrity.
func (p *CommandProgressRequest) Validate() error {
	if strings.TrimSpace(p.CommandID) == "" {
		return errors.New("commandId is required in progress")
	}
	if strings.TrimSpace(p.Stage) == "" {
		return errors.New("stage is required in progress")
	}
	return nil
}

// CommandStatusRequest is sent by the agent to report execution status (completion/failure).
type CommandStatusRequest struct {
	Status       string         `json:"status"`
	Result       map[string]any `json:"result"`
	ErrorCode    *string        `json:"errorCode"`
	ErrorMessage *string        `json:"errorMessage"`
}

// CommandCompletionRequest provides a strongly typed completion envelope.
type CommandCompletionRequest struct {
	CommandID    string         `json:"commandId"`
	Status       string         `json:"status"`
	Result       map[string]any `json:"result,omitempty"`
	ErrorCode    *string        `json:"errorCode,omitempty"`
	ErrorMessage *string        `json:"errorMessage,omitempty"`
	FinishedAt   time.Time      `json:"finishedAt"`
}

// Validate checks completion report integrity.
func (c *CommandCompletionRequest) Validate() error {
	if strings.TrimSpace(c.CommandID) == "" {
		return errors.New("commandId is required in completion")
	}
	if strings.TrimSpace(c.Status) == "" {
		return errors.New("status is required in completion")
	}
	return nil
}

// CommandError represents a structured, correlated error model.
type CommandError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Retryable bool           `json:"retryable"`
}
