package executor

import (
	"context"
	"log/slog"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// CommandCanceler provides the capability to abort an active command by its ID.
type CommandCanceler interface {
	CancelCommand(commandID string) bool
}

type cancelCommandRequest struct {
	CommandID       string `json:"commandId,omitempty"`
	TargetCommandID string `json:"targetCommandId,omitempty"`
}

func cancelCommandHandler(canceler CommandCanceler, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var req cancelCommandRequest
		if err := decodePayload(payload, &req); err != nil {
			return ExecutionResult{}, err
		}

		targetID := req.TargetCommandID
		if targetID == "" {
			targetID = req.CommandID
		}
		if targetID == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "targetCommandId or commandId is required")
		}

		cancelled := false
		if canceler != nil {
			cancelled = canceler.CancelCommand(targetID)
		}

		if log != nil {
			log.Info("command cancellation processed",
				slog.String("target_command_id", targetID),
				slog.Bool("cancelled", cancelled),
			)
		}

		return ExecutionResult{
			Output: map[string]any{
				"targetCommandId": targetID,
				"cancelled":       cancelled,
				"operation":       protocol.OpCancelCommand,
			},
		}, nil
	})
}
