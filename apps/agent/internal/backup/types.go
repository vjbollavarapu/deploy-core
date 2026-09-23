package backup

import "time"

// Request contains the parameters needed to execute a logical PostgreSQL backup.
type Request struct {
	BackupID        string `json:"backupId"`
	DatabaseID      string `json:"databaseId"`
	DatabaseName    string `json:"databaseName"`
	Username        string `json:"username"`
	Password        string `json:"password"` // Passed securely via exec env, never logged
	Type            string `json:"type,omitempty"`
	DestinationType string `json:"destinationType,omitempty"`
}

// Result summarizes the completed backup artifact.
type Result struct {
	BackupID       string    `json:"backupId"`
	Checksum       string    `json:"checksum"`
	SizeBytes      int64     `json:"sizeBytes"`
	DestinationURI string    `json:"destinationUri"`
	DurationMs     int64     `json:"durationMs"`
	CompletedAt    time.Time `json:"completedAt"`
}
