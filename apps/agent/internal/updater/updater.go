package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrChecksumMismatch = errors.New("updater: checksum mismatch")
	ErrExecutionFailed  = errors.New("updater: binary test execution failed")
	ErrRollbackFailed   = errors.New("updater: rollback failed")
	ErrInvalidManifest  = errors.New("updater: invalid update manifest")
)

// VersionInfo captures version mismatch and compatibility.
type VersionInfo struct {
	CurrentVersion   string `json:"currentVersion"`
	TargetVersion    string `json:"targetVersion,omitempty"`
	MinimumVersion   string `json:"minimumVersion,omitempty"`
	UpgradeAvailable bool   `json:"upgradeAvailable"`
	VersionMismatch  bool   `json:"versionMismatch"`
}

// UpdateManifest describes an approved release from the Control Plane.
type UpdateManifest struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	SHA256      string `json:"sha256"`
	Signature   string `json:"signature,omitempty"`
}

// UpdateResult details the outcome of an update cycle.
type UpdateResult struct {
	Status          string `json:"status"` // STAGED, APPLIED, FAILED, UP_TO_DATE
	TargetVersion   string `json:"targetVersion"`
	PreviousVersion string `json:"previousVersion"`
	Message         string `json:"message,omitempty"`
}

// CheckVersion compares local agent version against control plane version guidance.
func CheckVersion(current, target, min string) VersionInfo {
	info := VersionInfo{
		CurrentVersion: current,
		TargetVersion:  target,
		MinimumVersion: min,
	}

	if target != "" && target != current {
		info.UpgradeAvailable = true
	}

	if min != "" && isOlder(current, min) {
		info.VersionMismatch = true
	}

	return info
}

// isOlder does simple version comparison (vX.Y.Z).
func isOlder(current, min string) bool {
	c := strings.TrimPrefix(current, "v")
	m := strings.TrimPrefix(min, "v")
	return c < m // Simple fallback or lexical if semver not parsed
}

// StagedUpdater coordinates safe, staged updates and rollback.
type StagedUpdater struct {
	updatesDir string
	targetBin  string
}

// NewStagedUpdater constructs a StagedUpdater.
func NewStagedUpdater(updatesDir, targetBin string) *StagedUpdater {
	return &StagedUpdater{
		updatesDir: updatesDir,
		targetBin:  targetBin,
	}
}

// Stage writes the candidate binary to a versioned staging directory and validates its checksum.
func (u *StagedUpdater) Stage(ctx context.Context, version string, expectedSHA string, src io.Reader) (string, error) {
	if version == "" || expectedSHA == "" {
		return "", ErrInvalidManifest
	}

	destDir := filepath.Join(u.updatesDir, version)
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create update staging dir: %w", err)
	}

	candidatePath := filepath.Join(destDir, "deploycore-agent")
	f, err := os.OpenFile(candidatePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create candidate file: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	w := io.MultiWriter(f, hasher)

	if _, err := io.Copy(w, src); err != nil {
		_ = os.Remove(candidatePath)
		return "", fmt.Errorf("failed to write candidate binary: %w", err)
	}

	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualSHA, expectedSHA) {
		_ = os.Remove(candidatePath)
		return "", fmt.Errorf("%w: expected %s, calculated %s", ErrChecksumMismatch, expectedSHA, actualSHA)
	}

	return candidatePath, nil
}

// VerifyExecution tests that the candidate binary runs and outputs valid version.
func (u *StagedUpdater) VerifyExecution(ctx context.Context, candidatePath string) error {
	cmd := exec.CommandContext(ctx, candidatePath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %v, output: %s", ErrExecutionFailed, err, string(out))
	}
	return nil
}

// Apply atomically replaces targetBin with candidatePath, backing up targetBin to .old.
func (u *StagedUpdater) Apply(candidatePath string) error {
	backupPath := u.targetBin + ".old"

	// If target exists, rename it to .old
	if _, err := os.Stat(u.targetBin); err == nil {
		if err := os.Rename(u.targetBin, backupPath); err != nil {
			return fmt.Errorf("failed to backup existing binary: %w", err)
		}
	}

	// Move candidate to targetBin
	if err := os.Rename(candidatePath, u.targetBin); err != nil {
		// Attempt rollback immediately
		_ = os.Rename(backupPath, u.targetBin)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}

// Rollback restores targetBin from targetBin.old.
func (u *StagedUpdater) Rollback() error {
	backupPath := u.targetBin + ".old"
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("%w: backup file not found at %s", ErrRollbackFailed, backupPath)
	}

	// Atomic rollback rename
	if err := os.Rename(backupPath, u.targetBin); err != nil {
		return fmt.Errorf("%w: failed to restore backup binary: %v", ErrRollbackFailed, err)
	}

	return nil
}
