package protocol

// DatabaseBootstrap is returned by GET /agents/databases/{id}/bootstrap.
// Callers must never log Password.
type DatabaseBootstrap struct {
	DatabaseID        string `json:"databaseId"`
	Engine            string `json:"engine"`
	EngineVersion     string `json:"engineVersion"`
	DatabaseName      string `json:"databaseName"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	StorageVolumeName string `json:"storageVolumeName"`
	VolumeProtected   bool   `json:"volumeProtected"`
	CPUMillis         *int   `json:"cpuMillis,omitempty"`
	MemoryBytes       *int64 `json:"memoryBytes,omitempty"`
}
