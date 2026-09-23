package stats

import "time"

// ContainerStats holds the complete runtime resource snapshot for a container.
type ContainerStats struct {
	ContainerID   string    `json:"containerId"`
	ContainerName string    `json:"containerName"`
	ApplicationID string    `json:"applicationId,omitempty"`
	RevisionID    string    `json:"revisionId,omitempty"`
	Status        string    `json:"status"`
	CPUPercent    float64   `json:"cpuPercent"`
	MemoryUsage   int64     `json:"memoryUsageBytes"`
	MemoryLimit   int64     `json:"memoryLimitBytes"`
	NetworkRx     int64     `json:"networkRxBytes"`
	NetworkTx     int64     `json:"networkTxBytes"`
	BlockRead     int64     `json:"blockReadBytes"`
	BlockWrite    int64     `json:"blockWriteBytes"`
	PidsCurrent   int64     `json:"pidsCurrent"`
	RestartCount  int       `json:"restartCount"`
	Timestamp     time.Time `json:"timestamp"`
}

// SamplerConfig defines configuration parameters for the periodic stats sampler.
type SamplerConfig struct {
	Interval           time.Duration
	MaxSamplesPerEntry int
}

// DefaultSamplerConfig returns standard production defaults.
func DefaultSamplerConfig() SamplerConfig {
	return SamplerConfig{
		Interval:           30 * time.Second,
		MaxSamplesPerEntry: 10,
	}
}
