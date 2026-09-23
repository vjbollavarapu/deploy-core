package backupstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

func TestLocalStorage_SaveOpenDelete(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	content := []byte("PostgreSQL database dump contents sample data")
	hasher := sha256.New()
	hasher.Write(content)
	expectedChecksum := hex.EncodeToString(hasher.Sum(nil))

	backupID := "bak_test_123"

	// 1. Save
	meta, err := storage.Save(context.Background(), backupID, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if meta.SizeBytes != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), meta.SizeBytes)
	}
	if meta.Checksum != expectedChecksum {
		t.Errorf("expected checksum %s, got %s", expectedChecksum, meta.Checksum)
	}

	// 2. Exists
	exists, err := storage.Exists(context.Background(), backupID)
	if err != nil || !exists {
		t.Fatalf("expected backup to exist, exists=%v, err=%v", exists, err)
	}

	// 3. Open
	r, err := storage.Open(context.Background(), backupID)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer r.Close()

	readBytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(readBytes, content) {
		t.Errorf("expected read bytes to match original content")
	}

	// 4. Delete
	if err := storage.Delete(context.Background(), backupID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	exists, err = storage.Exists(context.Background(), backupID)
	if err != nil || exists {
		t.Errorf("expected backup to no longer exist after Delete, exists=%v", exists)
	}
}

func TestLocalStorage_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	attacks := []string{
		"../../etc/passwd",
		"../foo",
		"/var/log/dump",
		"bak/../../root",
		"bak;rm -rf /",
	}

	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			_, err := storage.Save(context.Background(), attack, bytes.NewReader([]byte("test")))
			if err == nil {
				t.Errorf("expected path traversal attack %q to be rejected, got nil", attack)
			}

			_, err = storage.Open(context.Background(), attack)
			if err == nil {
				t.Errorf("expected Open for attack %q to be rejected, got nil", attack)
			}
		})
	}
}
