package restore

import "time"

// Request contains the parameters needed to execute a controlled PostgreSQL restore.
type Request struct {
	RestoreID         string `json:"restoreId"`
	BackupID          string `json:"backupId"`
	TargetDatabaseID  string `json:"targetDatabaseId"`
	DatabaseName      string `json:"databaseName"`
	Username          string `json:"username"`
	Password          string `json:"password"` // Passed securely via exec env, never logged
	ExpectedChecksum  string `json:"checksum,omitempty"`
	RequireValidation bool   `json:"requireValidation"`
}

// Result summarizes the completed restore operation.
type Result struct {
	RestoreID        string    `json:"restoreId"`
	BackupID         string    `json:"backupId"`
	TargetDatabaseID string    `json:"targetDatabaseId"`
	Restored         bool      `json:"restored"`
	ValidationPassed bool      `json:"validationPassed"`
	DurationMs       int64     `json:"durationMs"`
	CompletedAt      time.Time `json:"completedAt"`
}
