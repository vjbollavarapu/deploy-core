package executor

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
)

type recordingBuilder struct {
	dockerfile string
	context    []byte
	err        error
}

func (r *recordingBuilder) BuildImage(_ context.Context, opts docker.BuildImageOptions) (docker.BuildImageResult, error) {
	r.dockerfile = opts.Dockerfile
	if opts.Context != nil {
		body, _ := io.ReadAll(opts.Context)
		r.context = body
	}
	if r.err != nil {
		return docker.BuildImageResult{}, r.err
	}
	return docker.BuildImageResult{Status: "built", ImageID: "sha256:test"}, nil
}

type scriptedCloner struct {
	n    int
	err  error
	tree map[string]string
}

func (s *scriptedCloner) Clone(_ context.Context, dest, repositoryURL, branch string) (string, error) {
	s.n++
	if s.err != nil {
		return "", errors.New(s.err.Error() + " " + repositoryURL)
	}
	for rel, content := range s.tree {
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, []byte(content), 0600); err != nil {
			return "", err
		}
	}
	note := branch + "-" + strconv.Itoa(s.n)
	if err := os.WriteFile(filepath.Join(dest, "clone.txt"), []byte(note), 0600); err != nil {
		return "", err
	}
	return "commit-" + note, nil
}

func TestGitBuildUsesDockerfileProdInBackendContext(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	cloner := &scriptedCloner{tree: map[string]string{
		"apps/backend/Dockerfile.prod": "FROM alpine\n",
		"Dockerfile":                   "FROM scratch\n",
	}}
	builder := &recordingBuilder{}
	h := buildImageHandlerWith(nil, builder, cloner, nil, workspace.NewManager(root), nil)
	fetch := map[string]any{
		"phase":         "fetch_source",
		"deploymentId":  "dep-modulyn",
		"applicationId": "app-modulyn",
		"revisionId":    "rev-2",
		"repositoryUrl": "https://github.com/vjbollavarapu/modulyn",
		"gitBranch":     "main",
	}
	if _, err := h.Execute(context.Background(), fetch); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "dep-modulyn", ".deploycore-source.json")
	body, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"revisionId":"rev-2"`) || strings.Contains(string(body), "token") {
		t.Fatalf("marker=%s", body)
	}
	build := map[string]any{
		"phase":          "build",
		"deploymentId":   "dep-modulyn",
		"applicationId":  "app-modulyn",
		"revisionId":     "rev-2",
		"repositoryUrl":  "https://github.com/vjbollavarapu/modulyn",
		"gitBranch":      "main",
		"dockerfilePath": "Dockerfile.prod",
		"contextPath":    "apps/backend",
	}
	res, err := h.Execute(context.Background(), build)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output["built"] != true {
		t.Fatalf("output=%v", res.Output)
	}
	if builder.dockerfile != "Dockerfile.prod" {
		t.Fatalf("docker dockerfile=%q", builder.dockerfile)
	}
	headers := tarNames(t, builder.context)
	if !headers["Dockerfile.prod"] {
		t.Fatalf("context files=%v", headers)
	}
	if headers["Dockerfile"] {
		t.Fatal("root Dockerfile was included in the apps/backend context")
	}
	if _, err := os.Stat(filepath.Join(root, "dep-modulyn")); !os.IsNotExist(err) {
		t.Fatal("workspace remained after a successful build")
	}
}

func TestGitBuildDefaultsToDockerfileAtWorkspaceRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	cloner := &scriptedCloner{tree: map[string]string{"Dockerfile": "FROM alpine\n"}}
	builder := &recordingBuilder{}
	h := buildImageHandlerWith(nil, builder, cloner, nil, workspace.NewManager(root), nil)
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-default",
		"repositoryUrl": "https://github.com/acme/app", "gitBranch": "main",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "build", "deploymentId": "dep-default",
		"dockerfilePath": "Dockerfile", "contextPath": ".",
	}); err != nil {
		t.Fatal(err)
	}
	if builder.dockerfile != "Dockerfile" {
		t.Fatalf("dockerfile=%q", builder.dockerfile)
	}
}

func TestSourceFetchFailureCleansOnlyThatDeployment(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	mgr := workspace.NewManager(root)
	ok := &scriptedCloner{tree: map[string]string{"keep.txt": "other"}}
	h := buildImageHandlerWith(nil, nil, ok, nil, mgr, nil)
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-other",
		"repositoryUrl": "https://github.com/acme/other", "gitBranch": "main",
	}); err != nil {
		t.Fatal(err)
	}
	failing := &scriptedCloner{err: errors.New("repository not found")}
	h = buildImageHandlerWith(nil, nil, failing, nil, mgr, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-fail",
		"repositoryUrl": "https://github.com/acme/missing", "gitBranch": "main",
	})
	if err == nil || !strings.Contains(err.Error(), "SOURCE_FETCH_FAILED") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "https://github.com/acme/missing") {
		t.Fatalf("repository URL leaked in error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "dep-fail")); !os.IsNotExist(statErr) {
		t.Fatal("failed fetch left its workspace behind")
	}
	if _, statErr := os.Stat(filepath.Join(root, "dep-other", "keep.txt")); statErr != nil {
		t.Fatal(statErr)
	}
}

func TestRepeatedFetchDoesNotTouchAnotherDeployment(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	cloner := &scriptedCloner{tree: map[string]string{"Dockerfile": "FROM alpine\n"}}
	h := buildImageHandlerWith(nil, nil, cloner, nil, workspace.NewManager(root), nil)
	for _, id := range []string{"dep-a", "dep-b"} {
		if _, err := h.Execute(context.Background(), map[string]any{
			"phase": "fetch_source", "deploymentId": id,
			"repositoryUrl": "https://github.com/acme/app", "gitBranch": id,
		}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(filepath.Join(root, "dep-b", "clone.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-a",
		"repositoryUrl": "https://github.com/acme/app", "gitBranch": "dep-a",
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, "dep-b", "clone.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("other workspace changed from %q to %q", before, after)
	}
	got, err := os.ReadFile(filepath.Join(root, "dep-a", "clone.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "dep-a-3" {
		t.Fatalf("refetched workspace=%q", got)
	}
}

func TestBuildWithoutFetchedSourceDoesNotPackageEmptyTree(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	builder := &recordingBuilder{}
	h := buildImageHandlerWith(nil, builder, &scriptedCloner{}, nil, workspace.NewManager(root), nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"phase": "build", "deploymentId": "dep-empty",
		"dockerfilePath": "Dockerfile", "contextPath": ".",
	})
	if err == nil || !strings.Contains(err.Error(), "SOURCE_NOT_READY") {
		t.Fatalf("err=%v", err)
	}
	if builder.dockerfile != "" {
		t.Fatal("docker build ran without a source marker")
	}
}

func TestMissingDockerfileIsNotDaemonUnavailable(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	cloner := &scriptedCloner{tree: map[string]string{"apps/backend/README.md": "hi"}}
	builder := &recordingBuilder{err: &docker.AgentError{Code: docker.ErrCodeDaemonUnavailable, Message: "should not be called"}}
	h := buildImageHandlerWith(nil, builder, cloner, nil, workspace.NewManager(root), nil)
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-missing-df",
		"repositoryUrl": "https://github.com/acme/app", "gitBranch": "main",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.Execute(context.Background(), map[string]any{
		"phase": "build", "deploymentId": "dep-missing-df",
		"dockerfilePath": "Dockerfile.prod", "contextPath": "apps/backend",
	})
	if err == nil || !strings.Contains(err.Error(), "DOCKERFILE_NOT_FOUND") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "DOCKER_DAEMON_UNAVAILABLE") {
		t.Fatalf("missing dockerfile classified as daemon: %v", err)
	}
	if !strings.Contains(err.Error(), "Dockerfile.prod") {
		t.Fatalf("useful path missing: %v", err)
	}
	if builder.dockerfile != "" {
		t.Fatal("docker was invoked for a missing dockerfile")
	}
}

func TestDockerDaemonUnavailableClassification(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	cloner := &scriptedCloner{tree: map[string]string{"Dockerfile": "FROM alpine\n"}}
	builder := &recordingBuilder{err: &docker.AgentError{
		Code:    docker.ErrCodeDaemonUnavailable,
		Message: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock: connection refused",
	}}
	h := buildImageHandlerWith(nil, builder, cloner, nil, workspace.NewManager(root), nil)
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-daemon",
		"repositoryUrl": "https://github.com/acme/app", "gitBranch": "main",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.Execute(context.Background(), map[string]any{
		"phase": "build", "deploymentId": "dep-daemon",
		"dockerfilePath": "Dockerfile", "contextPath": ".",
	})
	if err == nil || !strings.Contains(err.Error(), "DOCKER_UNAVAILABLE") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "DOCKER_DAEMON_UNAVAILABLE") {
		t.Fatalf("daemon code was forwarded unchanged: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("underlying message lost: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "dep-daemon")); !os.IsNotExist(statErr) {
		t.Fatal("workspace remained after build failure")
	}
}

func TestSymlinkDockerfileCannotEscapeWorkspace(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	cloner := &scriptedCloner{tree: map[string]string{"Dockerfile": "FROM alpine\n"}}
	builder := &recordingBuilder{}
	h := buildImageHandlerWith(nil, builder, cloner, nil, workspace.NewManager(root), nil)
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-link",
		"repositoryUrl": "https://github.com/acme/app", "gitBranch": "main",
	}); err != nil {
		t.Fatal(err)
	}
	df := filepath.Join(root, "dep-link", "Dockerfile")
	if err := os.Remove(df); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", df); err != nil {
		t.Fatal(err)
	}
	_, err := h.Execute(context.Background(), map[string]any{
		"phase": "build", "deploymentId": "dep-link",
		"dockerfilePath": "Dockerfile", "contextPath": ".",
	})
	if err == nil || !strings.Contains(err.Error(), "INVALID_SOURCE_PATH") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "ACCESS_DENIED") || strings.Contains(err.Error(), "FILESYSTEM_ACCESS_DENIED") {
		t.Fatalf("symlink escape used the wrong code: %v", err)
	}
	if builder.dockerfile != "" {
		t.Fatal("docker received an escaped dockerfile")
	}
}

func TestDockerfileSymlinkInsideWorkspaceIsAllowed(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspaces")
	cloner := &scriptedCloner{tree: map[string]string{
		"apps/backend/image.Dockerfile": "FROM alpine\n",
	}}
	builder := &recordingBuilder{}
	h := buildImageHandlerWith(nil, builder, cloner, nil, workspace.NewManager(root), nil)
	if _, err := h.Execute(context.Background(), map[string]any{
		"phase": "fetch_source", "deploymentId": "dep-inner-link",
		"repositoryUrl": "https://github.com/acme/app", "gitBranch": "main",
	}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "dep-inner-link", "apps", "backend", "Dockerfile.prod")
	if err := os.Symlink("image.Dockerfile", link); err != nil {
		t.Fatal(err)
	}
	res, err := h.Execute(context.Background(), map[string]any{
		"phase": "build", "deploymentId": "dep-inner-link",
		"dockerfilePath": "Dockerfile.prod", "contextPath": "apps/backend",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output["built"] != true || builder.dockerfile != "Dockerfile.prod" {
		t.Fatalf("built=%v dockerfile=%q", res.Output["built"], builder.dockerfile)
	}
}

func tarNames(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[hdr.Name] = true
	}
	return names
}
