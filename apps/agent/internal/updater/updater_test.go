package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckVersion(t *testing.T) {
	t.Run("UpToDate", func(t *testing.T) {
		info := CheckVersion("1.2.0", "1.2.0", "1.0.0")
		if info.UpgradeAvailable {
			t.Errorf("expected UpgradeAvailable=false")
		}
		if info.VersionMismatch {
			t.Errorf("expected VersionMismatch=false")
		}
	})

	t.Run("UpgradeAvailable", func(t *testing.T) {
		info := CheckVersion("1.2.0", "1.3.0", "1.0.0")
		if !info.UpgradeAvailable {
			t.Errorf("expected UpgradeAvailable=true")
		}
		if info.VersionMismatch {
			t.Errorf("expected VersionMismatch=false")
		}
	})

	t.Run("VersionMismatchBelowMinimum", func(t *testing.T) {
		info := CheckVersion("1.0.0", "2.0.0", "1.5.0")
		if !info.VersionMismatch {
			t.Errorf("expected VersionMismatch=true")
		}
	})
}

func TestStagedUpdater_StageApplyRollback(t *testing.T) {
	tempDir := t.TempDir()
	updatesDir := filepath.Join(tempDir, "updates")
	binDir := filepath.Join(tempDir, "bin")
	_ = os.MkdirAll(binDir, 0755)

	targetBin := filepath.Join(binDir, "deploycore-agent")
	_ = os.WriteFile(targetBin, []byte("#!/bin/sh\necho v1.0.0\n"), 0755)

	updater := NewStagedUpdater(updatesDir, targetBin)

	// Test 1: Staging candidate with checksum
	candidateContent := []byte("#!/bin/sh\necho v1.1.0\n")
	hasher := sha256.New()
	hasher.Write(candidateContent)
	expectedSHA := hex.EncodeToString(hasher.Sum(nil))

	stagedPath, err := updater.Stage(context.Background(), "v1.1.0", expectedSHA, bytes.NewReader(candidateContent))
	if err != nil {
		t.Fatalf("unexpected stage error: %v", err)
	}

	// Verify staged file
	stagedBytes, _ := os.ReadFile(stagedPath)
	if string(stagedBytes) != string(candidateContent) {
		t.Fatalf("staged content mismatch")
	}

	// Test 2: Stage fails on checksum mismatch
	_, err = updater.Stage(context.Background(), "v1.2.0", "invalidsha", bytes.NewReader(candidateContent))
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got %v", err)
	}

	// Test 3: Apply
	if err := updater.Apply(stagedPath); err != nil {
		t.Fatalf("failed to apply: %v", err)
	}

	// Check new target content
	appliedBytes, _ := os.ReadFile(targetBin)
	if string(appliedBytes) != string(candidateContent) {
		t.Errorf("expected new content in targetBin")
	}

	// Check backup file exists
	backupPath := targetBin + ".old"
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("expected backup file at %s", backupPath)
	}

	// Test 4: Rollback
	if err := updater.Rollback(); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	rolledBackBytes, _ := os.ReadFile(targetBin)
	if string(rolledBackBytes) != "#!/bin/sh\necho v1.0.0\n" {
		t.Errorf("expected original content after rollback")
	}
}
