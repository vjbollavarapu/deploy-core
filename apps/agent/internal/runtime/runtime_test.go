package runtime_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/runtime"
)

func TestInitPaths_ExplicitConfig(t *testing.T) {
	tmpData := t.TempDir()

	cfg := config.Config{
		DataDir:        tmpData,
		CredentialPath: filepath.Join(tmpData, "custom-creds.json"),
	}

	paths, err := runtime.InitPaths(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if paths.DataDir != tmpData {
		t.Errorf("expected DataDir %s, got %s", tmpData, paths.DataDir)
	}
	if paths.CredentialPath != filepath.Join(tmpData, "custom-creds.json") {
		t.Errorf("expected CredentialPath %s, got %s", filepath.Join(tmpData, "custom-creds.json"), paths.CredentialPath)
	}

	// Ensure directory was created
	stat, err := os.Stat(paths.DataDir)
	if err != nil {
		t.Fatalf("failed to stat DataDir: %v", err)
	}
	if !stat.IsDir() {
		t.Errorf("DataDir is not a directory")
	}
}

func TestInitPaths_Defaults(t *testing.T) {
	// Not testing full fallback logic since it depends on the host OS/user.
	// We'll just verify it initializes without error.
	cfg := config.Config{}

	paths, err := runtime.InitPaths(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if paths.DataDir == "" {
		t.Errorf("expected DataDir to be populated")
	}
	if paths.CredentialPath == "" {
		t.Errorf("expected CredentialPath to be populated")
	}
}
