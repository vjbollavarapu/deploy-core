package executor

import (
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// CommandState represents the lifecycle state of a command as tracked by the agent.
type CommandState string

const (
	StateReceived   CommandState = "RECEIVED"
	StateValidating CommandState = "VALIDATING"
	StateAccepted   CommandState = "ACCEPTED"
	StateRunning    CommandState = "RUNNING"
	StateCompleted  CommandState = "COMPLETED"
	StateFailed     CommandState = "FAILED"
	StateExpired    CommandState = "EXPIRED"
	StateRejected   CommandState = "REJECTED"
	StateCancelled  CommandState = "CANCELLED"
)

// ErrCode is a stable, agent-defined error code for command failures.
type ErrCode string

const (
	ErrCodeInvalidPayload      ErrCode = "INVALID_PAYLOAD"
	ErrCodeUnknownOperation    ErrCode = "UNKNOWN_OPERATION"
	ErrCodeServerMismatch      ErrCode = "SERVER_ID_MISMATCH"
	ErrCodeExpired             ErrCode = protocol.ErrCommandExpired
	ErrCodeDuplicate           ErrCode = protocol.ErrCommandReplayed
	ErrCodeSchemaVersion       ErrCode = "UNSUPPORTED_SCHEMA_VERSION"
	ErrCodeDockerError         ErrCode = "DOCKER_ERROR"
	ErrCodeCancelled           ErrCode = "COMMAND_CANCELLED"
	ErrCodeCapacityExceeded    ErrCode = "CAPACITY_EXCEEDED"
	ErrCodeInternalError       ErrCode = "INTERNAL_ERROR"
	ErrCodeConflict            ErrCode = "CONFLICT"
	ErrCodeValidation          ErrCode = "VALIDATION_FAILED"
	ErrCodeRoutingFailed       ErrCode = protocol.ErrRoutingFailed
	ErrCodeInsufficientDisk    ErrCode = protocol.ErrInsufficientDisk
	ErrCodeInsufficientMemory  ErrCode = protocol.ErrInsufficientMemory
	ErrCodeDockerUnavailable   ErrCode = protocol.ErrDockerUnavailable
	ErrCodeImageNotFound       ErrCode = protocol.ErrImageNotFound
	ErrCodeImagePullFailed     ErrCode = protocol.ErrImagePullFailed
	ErrCodeImageBuildFailed    ErrCode = protocol.ErrImageBuildFailed
	ErrCodeContainerNotFound   ErrCode = protocol.ErrContainerNotFound
	ErrCodeContainerCreateFail ErrCode = protocol.ErrContainerCreateFailed
	ErrCodeContainerStartFail  ErrCode = protocol.ErrContainerStartFailed
	ErrCodeContainerStopFail   ErrCode = protocol.ErrContainerStopFailed
	ErrCodeNetworkNotFound     ErrCode = protocol.ErrNetworkNotFound
	ErrCodeNetworkConflict     ErrCode = protocol.ErrNetworkConflict
	ErrCodeVolumeNotFound      ErrCode = protocol.ErrVolumeNotFound
	ErrCodeVolumeInUse         ErrCode = protocol.ErrVolumeInUse
	ErrCodeHealthCheckFailed   ErrCode = protocol.ErrHealthCheckFailed
	ErrCodeBackupFailed        ErrCode = protocol.ErrBackupFailed
	ErrCodeRestoreFailed       ErrCode = protocol.ErrRestoreFailed
	ErrCodeWorkspaceInvalid    ErrCode = protocol.ErrWorkspaceInvalid
	ErrCodeAccessDenied        ErrCode = protocol.ErrFilesystemAccessDenied
)

// ExecutionError is a structured error produced during command execution.
// It carries a stable ErrCode and is safe to send back to the Control Plane.
type ExecutionError struct {
	Code          ErrCode
	Message       string
	CorrelationID string
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Errorf creates a new ExecutionError with sanitized format.
func Errorf(code ErrCode, format string, args ...any) *ExecutionError {
	return &ExecutionError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// SanitizeMessage scrubs sensitive substrings such as credentials from error messages.
func SanitizeMessage(msg string, sensitiveValues ...string) string {
	res := msg
	for _, val := range sensitiveValues {
		if val != "" {
			res = strings.ReplaceAll(res, val, "[REDACTED]")
		}
	}
	return res
}

// ExecutionResult is the structured output of a completed command handler.
// Output is serialized and returned to the Control Plane via CommandStatusRequest.Result.
type ExecutionResult struct {
	Output map[string]any
}
