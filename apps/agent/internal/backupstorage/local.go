package backupstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var validIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// LocalStorage implements Storage using the agent's local filesystem under paths.BackupsDir.
type LocalStorage struct {
	baseDir string
}

// NewLocalStorage constructs a LocalStorage manager.
func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if strings.TrimSpace(baseDir) == "" {
		return nil, errors.New("baseDir cannot be empty")
	}
	clean := filepath.Clean(baseDir)
	if err := os.MkdirAll(clean, 0700); err != nil {
		return nil, fmt.Errorf("failed to create backups directory: %w", err)
	}
	return &LocalStorage{baseDir: clean}, nil
}

// resolvePath validates backupID and ensures the resolved file resides safely within baseDir.
func (s *LocalStorage) resolvePath(backupID string) (string, error) {
	cleanID := strings.TrimSpace(backupID)
	if cleanID == "" {
		return "", errors.New("backupID cannot be empty")
	}
	if !validIDRegex.MatchString(cleanID) || strings.Contains(cleanID, "..") || strings.Contains(cleanID, "/") || strings.Contains(cleanID, `\`) {
		return "", fmt.Errorf("invalid backup ID %q (path traversal forbidden)", backupID)
	}

	target := filepath.Join(s.baseDir, cleanID+".dump")
	rel, err := filepath.Rel(s.baseDir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("resolved path escapes backups base directory: %s", target)
	}
	return target, nil
}

// Save streams data from reader into a secure temporary file, computes sha256 checksum and size,
// and atomically renames to the final backup file.
func (s *LocalStorage) Save(ctx context.Context, backupID string, r io.Reader) (*BackupMeta, error) {
	destPath, err := s.resolvePath(backupID)
	if err != nil {
		return nil, err
	}

	tmpPath := destPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging backup file: %w", err)
	}

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	written, copyErr := io.Copy(mw, r)
	_ = f.Close()

	if copyErr != nil || ctx.Err() != nil {
		_ = os.Remove(tmpPath)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to write backup stream: %w", copyErr)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to finalize backup file: %w", err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	uri := s.ResolveURI(backupID)

	return &BackupMeta{
		BackupID:  backupID,
		URI:       uri,
		SizeBytes: written,
		Checksum:  checksum,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Open opens a stored backup file for reading.
func (s *LocalStorage) Open(ctx context.Context, backupID string) (io.ReadCloser, error) {
	path, err := s.resolvePath(backupID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Delete removes a stored backup artifact.
func (s *LocalStorage) Delete(ctx context.Context, backupID string) error {
	path, err := s.resolvePath(backupID)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// Exists checks whether a backup artifact exists.
func (s *LocalStorage) Exists(ctx context.Context, backupID string) (bool, error) {
	path, err := s.resolvePath(backupID)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ResolveURI returns the platform-managed URI for the backup artifact.
func (s *LocalStorage) ResolveURI(backupID string) string {
	path, err := s.resolvePath(backupID)
	if err != nil {
		return ""
	}
	return "local://" + path
}
