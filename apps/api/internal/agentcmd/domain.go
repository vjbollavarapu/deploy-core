package agentcmd

import (
	"encoding/json"
	"time"

	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

// Constants replaced by protocol-go

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
		protocol.OpDeployRevision, protocol.OpStopContainer, protocol.OpStartContainer, protocol.OpRestartContainer, protocol.OpRemoveContainer,
		protocol.OpFetchLogs, protocol.OpStreamLogs, protocol.OpBuildImage, protocol.OpPullImage,
		protocol.OpCreateNetwork, protocol.OpRemoveNetwork, protocol.OpInspectNetwork,
		protocol.OpCreateVolume, protocol.OpRemoveVolume, protocol.OpAttachVolume, protocol.OpDetachVolume, protocol.OpInspectVolume,
		protocol.OpRunHealthCheck, protocol.OpCreateBackup, protocol.OpRestoreBackup,
		protocol.OpProvisionDatabase, protocol.OpStartDatabase, protocol.OpStopDatabase,
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
