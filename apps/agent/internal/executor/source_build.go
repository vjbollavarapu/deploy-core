package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/buildlogs"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/safety"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

const sourceMarkerName = ".deploycore-source.json"

type sourceMarker struct {
	DeploymentID  string `json:"deploymentId"`
	RevisionID    string `json:"revisionId"`
	RepositoryURL string `json:"repositoryUrl"`
	Branch        string `json:"branch"`
	Commit        string `json:"commit"`
}

type imageBuilder interface {
	BuildImage(ctx context.Context, opts docker.BuildImageOptions) (docker.BuildImageResult, error)
}

func buildImageHandler(cli *docker.Client, tr transport.Client, wsMgr *workspace.Manager, log *slog.Logger) Handler {
	var builder imageBuilder
	if cli != nil {
		builder = cli
	}
	return buildImageHandlerWith(cli, builder, goGitCloner{}, tr, wsMgr, log)
}

func buildImageHandlerWith(cli *docker.Client, builder imageBuilder, cloner gitCloner, tr transport.Client, wsMgr *workspace.Manager, log *slog.Logger) Handler {
	if cloner == nil {
		cloner = goGitCloner{}
	}
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p protocol.BuildImagePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		if err := p.Validate(); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "%s", err.Error())
		}
		if wsMgr == nil {
			return ExecutionResult{}, Errorf(ErrCodeInternalError, "workspace manager not initialized")
		}
		if p.Phase == protocol.BuildPhaseFetchSource {
			return fetchBuildSource(ctx, p, cloner, wsMgr, log)
		}
		return buildFetchedSource(ctx, p, cli, builder, tr, wsMgr, log)
	})
}

func fetchBuildSource(ctx context.Context, p protocol.BuildImagePayload, cloner gitCloner, wsMgr *workspace.Manager, log *slog.Logger) (ExecutionResult, error) {
	ws, err := wsMgr.Reset(p.DeploymentID)
	if err != nil {
		return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "invalid deployment workspace: %v", err)
	}
	failed := true
	defer func() {
		if failed {
			_ = ws.Cleanup()
		}
	}()
	if err := ws.CheckDiskSpace(workspace.DefaultMinFreeDiskBytes); err != nil {
		return ExecutionResult{}, Errorf(ErrCodeCapacityExceeded, "disk space check failed: %v", err)
	}

	commit := ""
	switch {
	case strings.TrimSpace(p.RepositoryURL) != "":
		if log != nil {
			host := ""
			if parsed, parseErr := url.Parse(p.RepositoryURL); parseErr == nil {
				host = parsed.Host
			}
			log.Info("fetching git source",
				slog.String("deploymentId", p.DeploymentID),
				slog.String("host", host),
				slog.String("branch", p.GitBranch),
			)
		}
		commit, err = cloner.Clone(ctx, ws.Dir, p.RepositoryURL, p.GitBranch)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeSourceFetchFailed, "%s", SanitizeMessage(err.Error(), p.RepositoryURL))
		}
	case p.Archive != "":
		archiveBytes, err := base64.StdEncoding.DecodeString(p.Archive)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to decode base64 archive: %v", err)
		}
		if err := ws.ExtractArchive(bytes.NewReader(archiveBytes), p.ArchiveFormat, workspace.ExtractOptions{
			StripComponents: p.StripComponents,
		}); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to extract source archive: %v", err)
		}
	case len(p.Files) > 0:
		if err := ws.MaterializeFiles(p.Files); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "failed to materialize source files: %v", err)
		}
	default:
		return ExecutionResult{}, Errorf(ErrCodeSourceFetchFailed, "repositoryUrl is required to fetch source")
	}
	if err := writeSourceMarker(ws, p, commit); err != nil {
		return ExecutionResult{}, Errorf(ErrCodeSourceFetchFailed, "failed to record source: %v", err)
	}
	failed = false
	return ExecutionResult{Output: map[string]any{
		"deploymentId": p.DeploymentID,
		"sourceReady":  true,
		"phase":        protocol.BuildPhaseFetchSource,
		"commit":       commit,
	}}, nil
}

func buildFetchedSource(ctx context.Context, p protocol.BuildImagePayload, cli *docker.Client, builder imageBuilder, tr transport.Client, wsMgr *workspace.Manager, log *slog.Logger) (ExecutionResult, error) {
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
	ws, err := wsMgr.Get(p.DeploymentID)
	if err != nil {
		return ExecutionResult{}, Errorf(ErrCodeSourceNotReady, "source workspace is not ready")
	}
	defer func() { _ = ws.Cleanup() }()

	marker, err := readSourceMarker(ws)
	if err != nil || marker.DeploymentID != p.DeploymentID || (p.RevisionID != "" && marker.RevisionID != "" && marker.RevisionID != p.RevisionID) {
		return ExecutionResult{}, Errorf(ErrCodeSourceNotReady, "fetched source does not match this deployment")
	}
	contextDir, err := ws.ResolvePath(p.ContextPath)
	if err != nil {
		return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "invalid build context")
	}
	if err := stayInside(ws.Dir, contextDir); err != nil {
		return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "invalid build context")
	}
	dockerfilePath, err := resolveDockerfile(contextDir, p.DockerfilePath)
	if err != nil {
		return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "invalid dockerfile path")
	}
	if err := lexicalInside(ws.Dir, dockerfilePath); err != nil {
		return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "invalid dockerfile path")
	}
	info, err := os.Lstat(dockerfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ExecutionResult{}, Errorf(ErrCodeDockerfileNotFound, "Cannot locate specified Dockerfile: %s", p.DockerfilePath)
		}
		return ExecutionResult{}, Errorf(ErrCodeDockerfileNotFound, "failed to read dockerfile: %s", p.DockerfilePath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := symlinkTargetInside(ws.Dir, dockerfilePath); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "invalid dockerfile path")
		}
	}
	if info.IsDir() {
		return ExecutionResult{}, Errorf(ErrCodeDockerfileNotFound, "Cannot locate specified Dockerfile: %s", p.DockerfilePath)
	}
	if builder == nil {
		return ExecutionResult{}, Errorf(ErrCodeDockerUnavailable, "docker daemon is unavailable")
	}

	tarContext, err := ws.PackageContext(p.ContextPath, workspace.DefaultMaxContextBytes)
	if err != nil {
		if errors.Is(err, workspace.ErrSymlinkEscape) || errors.Is(err, workspace.ErrPathTraversal) {
			return ExecutionResult{}, Errorf(ErrCodeInvalidSourcePath, "invalid build context")
		}
		return ExecutionResult{}, Errorf(ErrCodeImageBuildFailed, "failed to package build context: %v", err)
	}

	target := p.Target
	if target == "" {
		target = p.TargetStage
	}
	buildArgs := make(map[string]*string, len(p.BuildArgs))
	for key, value := range p.BuildArgs {
		copied := value
		buildArgs[key] = &copied
	}
	var timeout time.Duration
	if p.TimeoutSeconds != nil && *p.TimeoutSeconds > 0 {
		timeout = time.Duration(*p.TimeoutSeconds) * time.Second
	}
	var revisionID *string
	if p.RevisionID != "" {
		revisionID = &p.RevisionID
	}
	streamer := buildlogs.NewStreamer(ctx, buildlogs.StreamOptions{
		ApplicationID:   p.ApplicationID,
		DeploymentID:    p.DeploymentID,
		RevisionID:      revisionID,
		BufferSize:      1000,
		BatchSize:       25,
		FlushInterval:   200 * time.Millisecond,
		RetentionPolicy: buildlogs.RetentionRetain,
	}, tr, log)
	defer streamer.Close()

	res, err := builder.BuildImage(ctx, docker.BuildImageOptions{
		Context:    tarContext,
		Dockerfile: p.DockerfilePath,
		Tags:       p.Tags,
		BuildArgs:  buildArgs,
		Target:     target,
		Platform:   p.Platform,
		NoCache:    strings.EqualFold(p.CachePolicy, "no-cache") || strings.EqualFold(p.CachePolicy, "nocache"),
		ProgressFn: streamer.Push,
		Timeout:    timeout,
	})
	if err != nil {
		return ExecutionResult{}, classifyBuildFailure(err)
	}
	return ExecutionResult{Output: map[string]any{
		"imageId":         res.ImageID,
		"digest":          res.Digest,
		"size":            res.Size,
		"buildDuration":   res.BuildDuration.String(),
		"buildDurationMs": res.BuildDurationMs,
		"tags":            res.Tags,
		"status":          res.Status,
		"deploymentId":    p.DeploymentID,
		"built":           true,
	}}, nil
}

func classifyBuildFailure(err error) *ExecutionError {
	var agentErr *docker.AgentError
	if errors.As(err, &agentErr) {
		switch agentErr.Code {
		case docker.ErrCodeDockerfileNotFound:
			return Errorf(ErrCodeDockerfileNotFound, "%s", agentErr.Message)
		case docker.ErrCodeDaemonUnavailable:
			return Errorf(ErrCodeDockerUnavailable, "%s", agentErr.Message)
		default:
			if dockerfileText(agentErr.Message) {
				return Errorf(ErrCodeDockerfileNotFound, "%s", agentErr.Message)
			}
			return Errorf(ErrCodeImageBuildFailed, "%s", agentErr.Message)
		}
	}
	msg := err.Error()
	if dockerfileText(msg) {
		return Errorf(ErrCodeDockerfileNotFound, "%s", msg)
	}
	return Errorf(ErrCodeImageBuildFailed, "%s", msg)
}

func dockerfileText(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "cannot locate specified dockerfile") ||
		strings.Contains(lower, "failed to read dockerfile") ||
		strings.Contains(lower, "dockerfile not found")
}

func writeSourceMarker(ws *workspace.Workspace, p protocol.BuildImagePayload, commit string) error {
	marker := sourceMarker{
		DeploymentID:  p.DeploymentID,
		RevisionID:    p.RevisionID,
		RepositoryURL: publicRepositoryURL(p.RepositoryURL),
		Branch:        p.GitBranch,
		Commit:        commit,
	}
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ws.Dir, sourceMarkerName), raw, 0600)
}

func readSourceMarker(ws *workspace.Workspace) (sourceMarker, error) {
	raw, err := os.ReadFile(filepath.Join(ws.Dir, sourceMarkerName))
	if err != nil {
		return sourceMarker{}, err
	}
	var marker sourceMarker
	if err := json.Unmarshal(raw, &marker); err != nil {
		return sourceMarker{}, err
	}
	if marker.DeploymentID == "" {
		return sourceMarker{}, errors.New("source marker is empty")
	}
	return marker, nil
}

func resolveDockerfile(contextDir, dockerfilePath string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(dockerfilePath)))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", workspace.ErrPathTraversal
	}
	target := filepath.Join(contextDir, cleaned)
	if err := lexicalInside(contextDir, target); err != nil {
		return "", err
	}
	return target, nil
}

func stayInside(root, target string) error {
	if err := lexicalInside(root, target); err != nil {
		return err
	}
	info, statErr := os.Lstat(target)
	if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	return symlinkTargetInside(root, target)
}

func lexicalInside(root, target string) error {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return workspace.ErrPathTraversal
	}
	return nil
}

func symlinkTargetInside(root, linkPath string) error {
	resolved, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = filepath.Clean(root)
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return workspace.ErrSymlinkEscape
	}
	return nil
}
