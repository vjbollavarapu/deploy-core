package protocol

import "time"

// LogIngestRequest is the payload sent by the agent to stream logs to the CP.
type LogIngestRequest struct {
	Kind          string          `json:"kind"`
	ApplicationID string          `json:"applicationId"`
	DeploymentID  *string         `json:"deploymentId"`
	RevisionID    *string         `json:"revisionId"`
	Entries       []LogIngestLine `json:"entries"`
}

// LogIngestLine represents a single log line or deployment event.
type LogIngestLine struct {
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Stage     string    `json:"stage,omitempty"`
}

// MetricIngestRequest is the payload sent by the agent to report metrics.
type MetricIngestRequest struct {
	Server     *ServerMetric     `json:"server"`
	Containers []ContainerMetric `json:"containers"`
}

// ServerMetric represents host-level telemetry from the agent.
type ServerMetric struct {
	CPUPercent       *float64       `json:"cpuPercent"`
	MemoryUsedBytes  *int64         `json:"memoryUsedBytes"`
	MemoryTotalBytes *int64         `json:"memoryTotalBytes"`
	DiskUsedBytes    *int64         `json:"diskUsedBytes"`
	DiskTotalBytes   *int64         `json:"diskTotalBytes"`
	Load1            *float64       `json:"load1"`
	Load5            *float64       `json:"load5"`
	Load15           *float64       `json:"load15"`
	UptimeSeconds    *int64         `json:"uptimeSeconds"`
	NetworkRxBytes   *int64         `json:"networkRxBytes"`
	NetworkTxBytes   *int64         `json:"networkTxBytes"`
	ContainerCount   *int           `json:"containerCount"`
	RecordedAt       *time.Time     `json:"recordedAt"`
	Payload          map[string]any `json:"payload"`
}

// ContainerMetric represents container-level telemetry from the agent.
type ContainerMetric struct {
	ContainerID      string         `json:"containerId"`
	ContainerName    string         `json:"containerName"`
	ApplicationID    *string        `json:"applicationId"`
	CPUPercent       *float64       `json:"cpuPercent"`
	MemoryUsedBytes  *int64         `json:"memoryUsedBytes"`
	MemoryLimitBytes *int64         `json:"memoryLimitBytes"`
	NetworkRxBytes   *int64         `json:"networkRxBytes"`
	NetworkTxBytes   *int64         `json:"networkTxBytes"`
	RestartCount     *int           `json:"restartCount"`
	Status           string         `json:"status"`
	RecordedAt       *time.Time     `json:"recordedAt"`
	Payload          map[string]any `json:"payload"`
}
