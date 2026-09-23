package backupstorage

import (
	"context"
	"io"
	"time"
)

// BackupMeta contains metadata about a saved backup artifact.
type BackupMeta struct {
	BackupID  string    `json:"backupId"`
	URI       string    `json:"uri"`
	SizeBytes int64     `json:"sizeBytes"`
	Checksum  string    `json:"checksum"` // SHA-256 hex string
	CreatedAt time.Time `json:"createdAt"`
}

// Storage is the abstraction for storing, retrieving, and validating backup archives.
type Storage interface {
	Save(ctx context.Context, backupID string, r io.Reader) (*BackupMeta, error)
	Open(ctx context.Context, backupID string) (io.ReadCloser, error)
	Delete(ctx context.Context, backupID string) error
	Exists(ctx context.Context, backupID string) (bool, error)
	ResolveURI(backupID string) string
}
