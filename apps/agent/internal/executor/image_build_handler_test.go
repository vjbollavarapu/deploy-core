package executor

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
)

func TestBuildImageHandler_RejectsArbitraryHostFilesystem(t *testing.T) {
	tmpDir := t.TempDir()
	wsMgr := workspace.NewManager(filepath.Join(tmpDir, "workspaces"))

	h := buildImageHandler(nil, nil, wsMgr, nil)

	// Attempt to set build context to /etc or traverse outside
	attacks := []string{
		"/etc",
		"../../../../etc",
		"/var/log",
		"../outside",
	}

	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			payload := map[string]any{
				"phase":          "build",
				"deploymentId":   "dep-attack-1",
				"dockerfilePath": "Dockerfile",
				"contextPath":    attack,
			}
			_, err := h.Execute(context.Background(), payload)
			if err == nil {
				t.Fatalf("expected error for contextPath %q, got nil", attack)
			}
			if !strings.Contains(err.Error(), "invalid build context") {
				t.Errorf("expected 'invalid build context' error, got: %v", err)
			}
		})
	}
}

func TestBuildImageHandler_RejectsArbitraryDockerfilePath(t *testing.T) {
	tmpDir := t.TempDir()
	wsMgr := workspace.NewManager(filepath.Join(tmpDir, "workspaces"))

	h := buildImageHandler(nil, nil, wsMgr, nil)

	attacks := []string{
		"/etc/passwd",
		"../../Dockerfile",
		"../outside.dockerfile",
	}

	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			payload := map[string]any{
				"phase":          "build",
				"deploymentId":   "dep-attack-df",
				"contextPath":    ".",
				"dockerfilePath": attack,
			}
			_, err := h.Execute(context.Background(), payload)
			if err == nil {
				t.Fatalf("expected error for dockerfilePath %q, got nil", attack)
			}
			if !strings.Contains(err.Error(), "invalid dockerfile path") {
				t.Errorf("expected 'invalid dockerfile path' error, got: %v", err)
			}
		})
	}
}

func TestBuildImageHandler_WorkspaceLifecycleAndRetention(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "workspaces")
	wsMgr := workspace.NewManager(wsRoot)

	depIDClean := "dep-clean-on-success"
	h := buildImageHandler(nil, nil, wsMgr, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"phase":        "fetch_source",
		"deploymentId": depIDClean,
		"files": map[string]any{
			"Dockerfile": "FROM alpine\n",
		},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsRoot, depIDClean, "Dockerfile")); err != nil {
		t.Fatalf("fetched source did not survive until build: %v", err)
	}
	_, err = h.Execute(context.Background(), map[string]any{
		"phase":           "build",
		"deploymentId":    depIDClean,
		"dockerfilePath":  "Dockerfile",
		"contextPath":     ".",
		"retentionPolicy": "clean_always",
	})
	if err == nil || !strings.Contains(err.Error(), "DOCKER_UNAVAILABLE") {
		t.Fatalf("expected docker unavailable, got %v", err)
	}

	cleanPath := filepath.Join(wsRoot, depIDClean)
	if _, err := os.Stat(cleanPath); !os.IsNotExist(err) {
		t.Errorf("expected workspace %q to be deleted after build failure", cleanPath)
	}
}

func TestBuildImageHandler_WorkspaceRetention_Retain(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "workspaces")
	wsMgr := workspace.NewManager(wsRoot)

	depIDRetain := "dep-retained"
	h := buildImageHandler(nil, nil, wsMgr, nil)
	_, err := h.Execute(context.Background(), map[string]any{
		"phase":        "fetch_source",
		"deploymentId": depIDRetain,
		"files": map[string]any{
			"Dockerfile": "FROM alpine\n",
		},
		"retentionPolicy": "retain",
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	retainPath := filepath.Join(wsRoot, depIDRetain)
	if stat, err := os.Stat(retainPath); err != nil || !stat.IsDir() {
		t.Fatalf("expected fetched workspace %q to remain, err: %v", retainPath, err)
	}
	_, err = h.Execute(context.Background(), map[string]any{
		"phase":           "build",
		"deploymentId":    depIDRetain,
		"dockerfilePath":  "Dockerfile",
		"contextPath":     ".",
		"retentionPolicy": "retain",
	})
	if err == nil {
		t.Fatal("expected build to fail without a docker client")
	}
	if _, err := os.Stat(retainPath); !os.IsNotExist(err) {
		t.Fatalf("expected workspace %q to be cleaned after build, err: %v", retainPath, err)
	}
}

func TestBuildImageHandler_FetchSourcePhase_SafeArchive(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "workspaces")
	wsMgr := workspace.NewManager(wsRoot)

	// Create a small base64 tar archive
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: "Dockerfile",
		Mode: 0600,
		Size: int64(len("FROM alpine\nCMD [\"echo\", \"source ready\"]\n")),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("FROM alpine\nCMD [\"echo\", \"source ready\"]\n"))
	_ = tw.Close()

	encodedArchive := base64.StdEncoding.EncodeToString(buf.Bytes())

	payload := map[string]any{
		"deploymentId":    "dep-fetch-src",
		"phase":           "fetch_source",
		"archive":         encodedArchive,
		"archiveFormat":   "tar",
		"retentionPolicy": "retain", // retain so we can verify the extracted file
	}

	h := buildImageHandler(nil, nil, wsMgr, nil)
	res, err := h.Execute(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected successful fetch_source, got: %v", err)
	}

	if res.Output["sourceReady"] != true {
		t.Errorf("expected sourceReady: true, got %v", res.Output["sourceReady"])
	}

	// Verify file was unpacked into the isolated workspace
	dfPath := filepath.Join(wsRoot, "dep-fetch-src", "Dockerfile")
	content, err := os.ReadFile(dfPath)
	if err != nil || !strings.Contains(string(content), "source ready") {
		t.Errorf("expected extracted Dockerfile content, got: %s (err: %v)", string(content), err)
	}
}
