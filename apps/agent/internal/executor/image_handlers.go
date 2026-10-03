package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/logs"
	"github.com/deploycore/deploy-core/apps/agent/internal/safety"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// --------------------------------------------------------------------------
// Image operation handlers
// --------------------------------------------------------------------------

type pullImagePayload struct {
	Image          string               `json:"image"`
	ImageReference string               `json:"imageReference,omitempty"`
	RegistryAuth   *docker.RegistryAuth `json:"registryAuth,omitempty"`

	// Execution context for progress streaming
	ApplicationID string  `json:"applicationId,omitempty"`
	DeploymentID  *string `json:"deploymentId,omitempty"`
	RevisionID    *string `json:"revisionId,omitempty"`
}

func pullImageHandler(cli *docker.Client, tr transport.Client, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p pullImagePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		ref := strings.TrimSpace(p.Image)
		if ref == "" {
			ref = strings.TrimSpace(p.ImageReference)
		}
		if ref == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "image or imageReference is required")
		}
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		// Pre-flight host resource safety check (Phase A25)
		safetyChecker := safety.NewChecker(safety.NewDefaultSystemChecker(cli), safety.DefaultThresholds(), log)
		if err := safetyChecker.ValidateExpensiveOperation(ctx); err != nil {
			var sErr *safety.SafetyError
			if errors.As(err, &sErr) {
				return ExecutionResult{}, Errorf(ErrCode(sErr.Code), "%s", sErr.Message)
			}
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "pre-flight safety validation failed: %v", err)
		}

		// Ensure registry credentials are scrubbed from memory when the handler exits
		defer func() {
			if p.RegistryAuth != nil {
				p.RegistryAuth.Zero()
			}
		}()

		var progressFn docker.PullProgressFunc
		if tr != nil && p.ApplicationID != "" {
			redactor := logs.NewRedactor(nil)
			progressFn = func(ev docker.PullProgressEvent) {
				msg := ev.Status
				if ev.ID != "" {
					msg = fmt.Sprintf("[%s] %s %s", ev.ID, ev.Status, ev.Progress)
				}
				msg = redactor.Redact(strings.TrimSpace(msg))
				_ = tr.SendLogs(ctx, protocol.LogIngestRequest{
					Kind:          "build",
					ApplicationID: p.ApplicationID,
					DeploymentID:  p.DeploymentID,
					RevisionID:    p.RevisionID,
					Entries: []protocol.LogIngestLine{
						{
							Stream:    "system",
							Message:   msg,
							Timestamp: ev.Timestamp,
						},
					},
				})
			}
		} else if log != nil {
			progressFn = func(ev docker.PullProgressEvent) {
				if ev.Status != "" {
					log.Debug("pull progress",
						slog.String("id", ev.ID),
						slog.String("status", ev.Status),
						slog.String("progress", ev.Progress),
					)
				}
			}
		}

		res, err := cli.PullImageWithOptions(ctx, docker.PullImageOptions{
			Ref:          ref,
			RegistryAuth: p.RegistryAuth,
			ProgressFn:   progressFn,
		})
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"image":          ref,
				"digest":         res.Digest,
				"imageId":        res.ImageID,
				"size":           res.Size,
				"pullDuration":   res.PullDuration.String(),
				"pullDurationMs": res.PullDurationMs,
				"status":         res.Status,
				"pulled":         true,
			},
		}, nil
	})
}
