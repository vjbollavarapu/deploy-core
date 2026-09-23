package runtime

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
)

// Paths holds the resolved runtime directories.
type Paths struct {
	DataDir        string
	CredentialPath string
	WorkspacesDir  string
	BackupsDir     string
}

// InitPaths resolves and initializes required agent directories safely.
func InitPaths(cfg config.Config) (Paths, error) {
	var p Paths

	if cfg.DataDir != "" {
		p.DataDir = cfg.DataDir
	} else {
		// Default path logic
		usr, err := user.Current()
		if err != nil {
			return p, fmt.Errorf("failed to get current user: %w", err)
		}
		if usr.Uid == "0" {
			p.DataDir = "/var/lib/deploycore-agent"
		} else {
			p.DataDir = filepath.Join(usr.HomeDir, ".deploycore-agent")
		}
	}

	p.WorkspacesDir = filepath.Join(p.DataDir, "workspaces")
	p.BackupsDir = filepath.Join(p.DataDir, "backups")

	if cfg.CredentialPath != "" {
		p.CredentialPath = cfg.CredentialPath
	} else {
		p.CredentialPath = filepath.Join(p.DataDir, "credentials.json")
	}

	// Create directories securely
	if err := os.MkdirAll(p.DataDir, 0700); err != nil {
		return p, fmt.Errorf("failed to initialize data directory: %w", err)
	}
	if err := os.MkdirAll(p.WorkspacesDir, 0700); err != nil {
		return p, fmt.Errorf("failed to initialize workspaces directory: %w", err)
	}
	if err := os.MkdirAll(p.BackupsDir, 0700); err != nil {
		return p, fmt.Errorf("failed to initialize backups directory: %w", err)
	}

	return p, nil
}
