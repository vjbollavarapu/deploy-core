package managedfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSafePathResolver_Resolve(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspaces")
	if err := os.MkdirAll(workspaceDir, 0700); err != nil {
		t.Fatal(err)
	}

	t.Run("ValidRelativePath", func(t *testing.T) {
		res, err := Resolve(workspaceDir, "project-1/src")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(workspaceDir, "project-1/src")
		if res != expected {
			t.Errorf("expected %s, got %s", expected, res)
		}
	})

	t.Run("ValidAbsolutePathUnderRoot", func(t *testing.T) {
		validAbs := filepath.Join(workspaceDir, "project-1/src")
		res, err := Resolve(workspaceDir, validAbs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != validAbs {
			t.Errorf("expected %s, got %s", validAbs, res)
		}
	})

	t.Run("PathTraversalDotDot", func(t *testing.T) {
		_, err := Resolve(workspaceDir, "../outside")
		if !errors.Is(err, ErrPathTraversal) {
			t.Errorf("expected ErrPathTraversal, got %v", err)
		}
	})

	t.Run("PathTraversalAbsoluteOutside", func(t *testing.T) {
		_, err := Resolve(workspaceDir, "/etc/passwd")
		if !errors.Is(err, ErrPathTraversal) {
			t.Errorf("expected ErrPathTraversal, got %v", err)
		}
	})

	t.Run("NullByteInjection", func(t *testing.T) {
		_, err := Resolve(workspaceDir, "safe/path\x00/danger")
		if !errors.Is(err, ErrPathTraversal) {
			t.Errorf("expected ErrPathTraversal, got %v", err)
		}
	})

	t.Run("SymlinkEscapeCheck", func(t *testing.T) {
		outsideDir := filepath.Join(tempDir, "outside")
		if err := os.MkdirAll(outsideDir, 0700); err != nil {
			t.Fatal(err)
		}

		symlinkPath := filepath.Join(workspaceDir, "escape_link")
		if err := os.Symlink(outsideDir, symlinkPath); err != nil {
			t.Fatal(err)
		}

		_, err := Resolve(workspaceDir, "escape_link/subfile")
		if !errors.Is(err, ErrSymlinkEscape) {
			t.Errorf("expected ErrSymlinkEscape, got %v", err)
		}
	})
}

func TestSafePathResolver_ResolveUnderAny(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	wsDir := filepath.Join(tempDir, "workspaces")
	backupDir := filepath.Join(tempDir, "backups")
	_ = os.MkdirAll(dataDir, 0700)
	_ = os.MkdirAll(wsDir, 0700)
	_ = os.MkdirAll(backupDir, 0700)

	roots := AllowedRoots{
		DataDir:       dataDir,
		WorkspacesDir: wsDir,
		BackupsDir:    backupDir,
	}
	resolver := NewResolver(roots)

	t.Run("ResolveInWorkspaces", func(t *testing.T) {
		path := filepath.Join(wsDir, "job-1")
		res, err := resolver.ResolveUnderAny(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != path {
			t.Errorf("expected %s, got %s", path, res)
		}
	})

	t.Run("ResolveInBackups", func(t *testing.T) {
		path := filepath.Join(backupDir, "db.dump")
		res, err := resolver.ResolveUnderAny(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != path {
			t.Errorf("expected %s, got %s", path, res)
		}
	})

	t.Run("RejectOutsideAnyRoot", func(t *testing.T) {
		_, err := resolver.ResolveUnderAny("/var/log/syslog")
		if !errors.Is(err, ErrAccessDenied) {
			t.Errorf("expected ErrAccessDenied, got %v", err)
		}
	})
}
