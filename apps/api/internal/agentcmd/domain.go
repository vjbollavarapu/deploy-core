package agentcmd

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SchemaVersion is the current command contract version.
const SchemaVersion = 1

const (
	OpDeployRevision   = "DEPLOY_REVISION"
	OpStopContainer    = "STOP_CONTAINER"
	OpStartContainer   = "START_CONTAINER"
	OpRestartContainer = "RESTART_CONTAINER"
	OpRemoveContainer  = "REMOVE_CONTAINER"
	OpFetchLogs        = "FETCH_LOGS"
	OpStreamLogs       = "STREAM_LOGS"
	OpBuildImage       = "BUILD_IMAGE"
	OpPullImage        = "PULL_IMAGE"
	OpCreateNetwork    = "CREATE_NETWORK"
	OpCreateVolume     = "CREATE_VOLUME"
	OpRemoveVolume     = "REMOVE_VOLUME"
	OpAttachVolume     = "ATTACH_VOLUME"
	OpDetachVolume     = "DETACH_VOLUME"
	OpInspectVolume    = "INSPECT_VOLUME"
	OpRunHealthCheck   = "RUN_HEALTH_CHECK"
	OpCreateBackup     = "CREATE_BACKUP"
	OpRestoreBackup    = "RESTORE_BACKUP"
	OpProvisionDatabase = "PROVISION_DATABASE"
	OpStartDatabase     = "START_DATABASE"
	OpStopDatabase      = "STOP_DATABASE"
)

const (
	StatusPending   = "pending"
	StatusAccepted  = "accepted"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"
)

// Command is a versionable, structured instruction for an agent.
// Payload must be structured JSON — never arbitrary shell text.
type Command struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ServerID       uuid.UUID
	Operation      string
	SchemaVersion  int
	Payload        map[string]any
	Status         string
	IssuedAt       time.Time
	ExpiresAt      time.Time
	RequestID      *string
	CorrelationID  *string
	IssuedBy       *uuid.UUID
	Result         map[string]any
	ErrorCode      *string
	ErrorMessage   *string
	AcceptedAt     *time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type IssueInput struct {
	ServerID      uuid.UUID
	Operation     string
	Payload       map[string]any
	CorrelationID string
	TTL           time.Duration
}

func AllowedOperations() []string {
	return []string{
		OpDeployRevision, OpStopContainer, OpStartContainer, OpRestartContainer, OpRemoveContainer,
		OpFetchLogs, OpStreamLogs, OpBuildImage, OpPullImage, OpCreateNetwork, OpCreateVolume,
		OpRemoveVolume, OpAttachVolume, OpDetachVolume, OpInspectVolume,
		OpRunHealthCheck, OpCreateBackup, OpRestoreBackup,
		OpProvisionDatabase, OpStartDatabase, OpStopDatabase,
	}
}

func IsAllowedOperation(op string) bool {
	for _, allowed := range AllowedOperations() {
		if op == allowed {
			return true
		}
	}
	return false
}

func payloadJSON(m map[string]any) []byte {
	if m == nil {
		m = map[string]any{}
	}
	b, _ := json.Marshal(m)
	return b
}
