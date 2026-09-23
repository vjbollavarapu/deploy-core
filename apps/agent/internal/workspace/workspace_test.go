package workspace_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
)

func TestManager_Create(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)

	// Valid ID
	ws, err := mgr.Create("dep-123-abc")
	if err != nil {
		t.Fatalf("expected valid workspace creation, got %v", err)
	}
	if ws.Dir != filepath.Join(root, "dep-123-abc") {
		t.Errorf("unexpected dir: %s", ws.Dir)
	}
	stat, err := os.Stat(ws.Dir)
	if err != nil || !stat.IsDir() {
		t.Fatalf("expected directory to exist: %v", err)
	}

	// Invalid IDs with path traversal attempts
	invalidIDs := []string{
		"../escape",
		"../../etc/passwd",
		"/absolute/path",
		"bad;id",
		"id with spaces",
	}
	for _, id := range invalidIDs {
		_, err := mgr.Create(id)
		if err == nil {
			t.Errorf("expected error for invalid ID %q, got nil", id)
		}
	}
}

func TestWorkspace_ResolvePath_Security(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-test")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	// Safe relative paths
	safe, err := ws.ResolvePath("src/app/Dockerfile")
	if err != nil {
		t.Fatalf("expected safe path to resolve, got %v", err)
	}
	if !strings.HasPrefix(safe, ws.Dir) {
		t.Errorf("expected resolved path inside workspace: %s", safe)
	}

	// Path traversal attacks
	attacks := []string{
		"../outside",
		"../../../../etc/shadow",
		"/etc/passwd",
		"src/../../..",
		"/var/log/syslog",
	}
	for _, attack := range attacks {
		_, err := ws.ResolvePath(attack)
		if err == nil {
			t.Errorf("expected path traversal error for %q, got nil", attack)
		}
	}
}

func TestWorkspace_MaterializeAndPackageContext(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-pkg")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	files := map[string]string{
		"Dockerfile":         "FROM alpine:latest\nCMD [\"echo\", \"hello\"]\n",
		"src/main.go":        "package main\nfunc main() {}\n",
		"config/config.json": "{\"env\":\"prod\"}\n",
	}

	if err := ws.MaterializeFiles(files); err != nil {
		t.Fatalf("failed to materialize files: %v", err)
	}

	// Package entire workspace as context
	reader, err := ws.PackageContext("", 10*1024*1024)
	if err != nil {
		t.Fatalf("failed to package context: %v", err)
	}

	// Verify tar contents
	tr := tar.NewReader(reader)
	found := make(map[string]bool)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed reading tar: %v", err)
		}
		found[hdr.Name] = true
	}

	for f := range files {
		if !found[f] {
			t.Errorf("expected file %q in tar archive", f)
		}
	}
}

func TestWorkspace_PackageContext_SizeLimit(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-limit")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	files := map[string]string{
		"large.dat": strings.Repeat("a", 2048),
	}
	_ = ws.MaterializeFiles(files)

	// Set limit lower than file size
	_, err = ws.PackageContext("", 500)
	if err == nil {
		t.Fatal("expected size limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWorkspace_Cleanup(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-cleanup")
	if err != nil {
		t.Fatal(err)
	}

	_ = ws.MaterializeFiles(map[string]string{"Dockerfile": "FROM scratch"})

	if err := ws.Cleanup(); err != nil {
		t.Fatalf("failed to cleanup workspace: %v", err)
	}

	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Errorf("expected workspace directory to be deleted, stat err: %v", err)
	}
}

func createTarArchive(t *testing.T, entries map[string]string, symlinks map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for name, content := range entries {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0600,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	for name, target := range symlinks {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0777,
			Typeflag: tar.TypeSymlink,
			Linkname: target,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func createZipArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWorkspace_ExtractArchive_Tar_Safe(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-tar-safe")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	archive := createTarArchive(t, map[string]string{
		"Dockerfile":  "FROM alpine\n",
		"src/main.py": "print('hello')\n",
	}, nil)

	err = ws.ExtractArchive(bytes.NewReader(archive), "tar", workspace.ExtractOptions{})
	if err != nil {
		t.Fatalf("expected successful tar extraction, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(ws.Dir, "Dockerfile"))
	if err != nil || string(content) != "FROM alpine\n" {
		t.Errorf("unexpected extracted content: %s, err: %v", string(content), err)
	}
}

func TestWorkspace_ExtractArchive_Zip_Safe(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-zip-safe")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	archive := createZipArchive(t, map[string]string{
		"package.json": "{\"name\":\"myapp\"}\n",
		"index.js":     "console.log('test');\n",
	})

	err = ws.ExtractArchive(bytes.NewReader(archive), "zip", workspace.ExtractOptions{})
	if err != nil {
		t.Fatalf("expected successful zip extraction, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(ws.Dir, "index.js"))
	if err != nil || string(content) != "console.log('test');\n" {
		t.Errorf("unexpected extracted content: %s, err: %v", string(content), err)
	}
}

func TestWorkspace_ExtractArchive_TarSlip_Rejected(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-tarslip")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	// Malicious archive with path traversal
	maliciousTar := createTarArchive(t, map[string]string{
		"../../../../etc/malicious.conf": "hacked\n",
	}, nil)

	err = ws.ExtractArchive(bytes.NewReader(maliciousTar), "tar", workspace.ExtractOptions{})
	if err == nil {
		t.Fatal("expected TarSlip path traversal error, got nil")
	}
	if !strings.Contains(err.Error(), "traversal") && !strings.Contains(err.Error(), "escapes") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWorkspace_ExtractArchive_SymlinkEscape_Rejected(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-symlink-escape")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	// Symlink pointing outside the workspace directory
	maliciousTar := createTarArchive(t, nil, map[string]string{
		"escape_link": "/etc/passwd",
	})

	err = ws.ExtractArchive(bytes.NewReader(maliciousTar), "tar", workspace.ExtractOptions{})
	if err == nil {
		t.Fatal("expected symlink escape error, got nil")
	}
	if !strings.Contains(err.Error(), "points outside workspace") && !strings.Contains(err.Error(), "symlink") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWorkspace_ExtractArchive_Limits(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-limits")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	// Test max extracted size
	tarBomb := createTarArchive(t, map[string]string{
		"large.bin": strings.Repeat("A", 10000),
	}, nil)

	err = ws.ExtractArchive(bytes.NewReader(tarBomb), "tar", workspace.ExtractOptions{
		MaxExtractedBytes: 500, // lower than 10000 bytes
	})
	if err == nil {
		t.Fatal("expected MaxExtractedBytes error, got nil")
	}

	// Test max file count
	manyFiles := make(map[string]string)
	for i := 0; i < 15; i++ {
		manyFiles[filepath.Join("files", strings.Repeat("f", i)+".txt")] = "test"
	}
	tarMany := createTarArchive(t, manyFiles, nil)

	err = ws.ExtractArchive(bytes.NewReader(tarMany), "tar", workspace.ExtractOptions{
		MaxFileCount: 5,
	})
	if err == nil {
		t.Fatal("expected MaxFileCount error, got nil")
	}
}

func TestWorkspace_ListFiles_NoArbitraryBrowsing(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)
	ws, err := mgr.Create("dep-browse")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	_ = ws.MaterializeFiles(map[string]string{
		"Dockerfile": "FROM scratch",
		"sub/a.txt":  "hello",
	})

	// Legitimate workspace listing
	files, err := ws.ListFiles("sub")
	if err != nil {
		t.Fatalf("expected safe list, got: %v", err)
	}
	if len(files) != 1 || files[0].Name != "a.txt" {
		t.Errorf("unexpected files list: %+v", files)
	}

	// Attempting to browse host filesystem
	escapes := []string{"..", "../../etc", "/etc"}
	for _, esc := range escapes {
		_, err := ws.ListFiles(esc)
		if err == nil {
			t.Errorf("expected error browsing %q, got nil", esc)
		}
	}
}

func TestManager_Prune(t *testing.T) {
	root := t.TempDir()
	mgr := workspace.NewManager(root)

	// Create workspace with immediate expiration
	ws, err := mgr.Create("dep-expired")
	if err != nil {
		t.Fatal(err)
	}

	_ = ws.SaveMetadata(workspace.RetentionCleanAlways, -1*time.Minute)

	removed, err := mgr.Prune(0)
	if err != nil {
		t.Fatalf("prune error: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed workspace, got %d", removed)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Errorf("expected expired workspace to be deleted")
	}
}
