package safety

import (
	"fmt"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

const (
	CodeInsufficientDisk   = protocol.ErrInsufficientDisk   // "INSUFFICIENT_DISK"
	CodeInsufficientMemory = protocol.ErrInsufficientMemory // "INSUFFICIENT_MEMORY"
	CodeDockerUnavailable  = protocol.ErrDockerUnavailable  // "DOCKER_UNAVAILABLE"
)

// SafetyError is a typed error containing the stable error code and diagnostic information.
type SafetyError struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	AvailableBytes int64  `json:"availableBytes,omitempty"`
	RequiredBytes  int64  `json:"requiredBytes,omitempty"`
}

func (e *SafetyError) Error() string {
	if e.RequiredBytes > 0 {
		return fmt.Sprintf("%s: %s (available: %d bytes, required: %d bytes)", e.Code, e.Message, e.AvailableBytes, e.RequiredBytes)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Thresholds defines resource thresholds below which expensive operations are aborted.
type Thresholds struct {
	MinDiskFreeBytes   int64   // Absolute minimum free disk in bytes (default: 2 GB)
	MinDiskFreePercent float64 // Minimum percent free (default: 5.0%)
	MinMemoryFreeBytes int64   // Minimum available host memory in bytes (default: 256 MB)
	CheckPath          string  // File path to measure disk space for (default: "/")
}

// DefaultThresholds returns safety limits configured to protect the host from resource exhaustion.
func DefaultThresholds() Thresholds {
	return Thresholds{
		MinDiskFreeBytes:   2 * 1024 * 1024 * 1024, // 2 GB
		MinDiskFreePercent: 5.0,                    // 5%
		MinMemoryFreeBytes: 256 * 1024 * 1024,      // 256 MB
		CheckPath:          "/",
	}
}
