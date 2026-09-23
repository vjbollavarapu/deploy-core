package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/buildlogs"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/logs"
	"github.com/deploycore/deploy-core/apps/agent/internal/safety"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
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

// buildImagePayload defines the structured request for building an image.
type buildImagePayload struct {
	DeploymentID    string            `json:"deploymentId"`
	ApplicationID   string            `json:"applicationId,omitempty"`
	RevisionID      *string           `json:"revisionId,omitempty"`
	Phase           string            `json:"phase,omitempty"` // "fetch_source" or "build"
	Archive         string            `json:"archive,omitempty"`
	ArchiveFormat   string            `json:"archiveFormat,omitempty"`
	StripComponents int               `json:"stripComponents,omitempty"`
	ContextPath     string            `json:"contextPath,omitempty"`
	Dockerfile      string            `json:"dockerfile,omitempty"`
	DockerfilePath  string            `json:"dockerfilePath,omitempty"`
	BuildArgs       map[string]string `json:"buildArgs,omitempty"`
	Target          string            `json:"target,omitempty"`
	TargetStage     string            `json:"targetStage,omitempty"`
	Platform        string            `json:"platform,omitempty"`
	CachePolicy     string            `json:"cachePolicy,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Files           map[string]string `json:"files,omitempty"`
	RetentionPolicy string            `json:"retentionPolicy,omitempty"`
	TimeoutSeconds  *int              `json:"timeoutSeconds,omitempty"`
}

// buildImageHandler handles building Docker images inside an isolated deployment workspace.
func buildImageHandler(cli *docker.Client, tr transport.Client, wsMgr *workspace.Manager, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p buildImagePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		deploymentID := strings.TrimSpace(p.DeploymentID)
		if deploymentID == "" {
			deploymentID = uuid.New().String()
		}

		// Pre-flight host resource safety check (Phase A25)
		if cli != nil {
			safetyChecker := safety.NewChecker(safety.NewDefaultSystemChecker(cli), safety.DefaultThresholds(), log)
			if err := safetyChecker.ValidateExpensiveOperation(ctx); err != nil {
				var sErr *safety.SafetyError
				if errors.As(err, &sErr) {
					return ExecutionResult{}, Errorf(ErrCode(sErr.Code), "%s", sErr.Message)
				}
				return ExecutionResult{}, Errorf(ErrCodeDockerError, "pre-flight safety validation failed: %v", err)
			}
		}

		if wsMgr == nil {
			return ExecutionResult{}, Errorf(ErrCodeInternalError, "workspace manager not initialized")
		}

		// 1. Create isolated deployment workspace: /var/lib/deploycore-agent/workspaces/<deployment-id>
		ws, err := wsMgr.Create(deploymentID)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "invalid deployment workspace: %v", err)
		}

		// 2. Clean failed workspaces and honor retention policy
		var execErr error
		defer func() {
			retPolicy := workspace.RetentionPolicy(p.RetentionPolicy)
			if retPolicy == workspace.RetentionRetain {
				_ = ws.SaveMetadata(workspace.RetentionRetain, workspace.DefaultRetentionTTL)
				return
			}
			// Clean on failure or if clean_always / default
			if execErr != nil || retPolicy != workspace.RetentionCleanOnSuccess {
				_ = ws.Cleanup()
			}
		}()

		// 3. Protect disk space
		if err := ws.CheckDiskSpace(workspace.DefaultMinFreeDiskBytes); err != nil {
			execErr = err
			return ExecutionResult{}, Errorf(ErrCodeCapacityExceeded, "disk space check failed: %v", err)
		}

		// 4. Fetch/materialize source
		if p.Archive != "" {
			archiveBytes, err := base64.StdEncoding.DecodeString(p.Archive)
			if err != nil {
				execErr = err
				return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to decode base64 archive: %v", err)
			}
			if err := ws.ExtractArchive(bytes.NewReader(archiveBytes), p.ArchiveFormat, workspace.ExtractOptions{
				StripComponents: p.StripComponents,
			}); err != nil {
				execErr = err
				return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to extract source archive: %v", err)
			}
		}

		if len(p.Files) > 0 {
			if err := ws.MaterializeFiles(p.Files); err != nil {
				execErr = err
				return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to materialize source files: %v", err)
			}
		}

		// If inline Dockerfile content is provided directly and not in files map:
		if p.Dockerfile != "" && (p.Files == nil || p.Files["Dockerfile"] == "") {
			dfName := p.DockerfilePath
			if dfName == "" {
				dfName = "Dockerfile"
			}
			if err := ws.MaterializeFiles(map[string]string{dfName: p.Dockerfile}); err != nil {
				execErr = err
				return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to materialize Dockerfile: %v", err)
			}
		}

		// If phase is fetch_source, stop here after source is ready
		if strings.EqualFold(p.Phase, "fetch_source") {
			return ExecutionResult{
				Output: map[string]any{
					"deploymentId": deploymentID,
					"sourceReady":  true,
					"phase":        "fetch_source",
				},
			}, nil
		}

		// 5. Validate context path - NEVER allow arbitrary host filesystem
		contextSubpath := p.ContextPath
		if _, err := ws.ResolvePath(contextSubpath); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "invalid build context: %v", err)
		}

		// 6. Validate Dockerfile path within context
		dockerfile := p.DockerfilePath
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		if _, err := ws.ResolvePath(filepath.Join(contextSubpath, dockerfile)); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "invalid dockerfile path: %v", err)
		}

		// 7. Package context tar archive strictly from approved workspace
		tarContext, err := ws.PackageContext(contextSubpath, workspace.DefaultMaxContextBytes)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to package build context: %v", err)
		}

		// 8. Prepare build options
		target := p.Target
		if target == "" {
			target = p.TargetStage
		}

		noCache := false
		if strings.EqualFold(p.CachePolicy, "no-cache") || strings.EqualFold(p.CachePolicy, "nocache") {
			noCache = true
		}

		buildArgs := make(map[string]*string, len(p.BuildArgs))
		for k, v := range p.BuildArgs {
			val := v
			buildArgs[k] = &val
		}

		var timeout time.Duration
		if p.TimeoutSeconds != nil && *p.TimeoutSeconds > 0 {
			timeout = time.Duration(*p.TimeoutSeconds) * time.Second
		}

		// 9. Stream build output with structured stage identification and bounded buffering
		streamer := buildlogs.NewStreamer(ctx, buildlogs.StreamOptions{
			ApplicationID:   p.ApplicationID,
			DeploymentID:    p.DeploymentID,
			RevisionID:      p.RevisionID,
			BufferSize:      1000,
			BatchSize:       25,
			FlushInterval:   200 * time.Millisecond,
			RetentionPolicy: buildlogs.RetentionRetain,
		}, tr, log)
		defer streamer.Close()

		progressFn := func(ev docker.BuildProgressEvent) {
			streamer.Push(ev)
		}

		// 10. Execute build
		res, err := cli.BuildImage(ctx, docker.BuildImageOptions{
			Context:    tarContext,
			Dockerfile: dockerfile,
			Tags:       p.Tags,
			BuildArgs:  buildArgs,
			Target:     target,
			Platform:   p.Platform,
			NoCache:    noCache,
			ProgressFn: progressFn,
			Timeout:    timeout,
		})
		if err != nil {
			execErr = err
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"imageId":         res.ImageID,
				"digest":          res.Digest,
				"size":            res.Size,
				"buildDuration":   res.BuildDuration.String(),
				"buildDurationMs": res.BuildDurationMs,
				"tags":            res.Tags,
				"status":          res.Status,
				"deploymentId":    deploymentID,
				"built":           true,
			},
		}, nil
	})
}
