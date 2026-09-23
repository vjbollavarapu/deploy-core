package safety

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemResourceChecker provides low-level host metrics and Docker connectivity checks.
type SystemResourceChecker interface {
	CheckDiskSpace(ctx context.Context, path string) (freeBytes, totalBytes int64, err error)
	CheckMemory(ctx context.Context) (availableBytes, totalBytes int64, err error)
	CheckDockerHealth(ctx context.Context) error
}

// DockerPinger is the connectivity check interface.
type DockerPinger interface {
	CheckConnectivity(ctx context.Context) error
}

// DefaultSystemChecker uses gopsutil and the Docker client to inspect local host resources.
type DefaultSystemChecker struct {
	dockerCli DockerPinger
}

// NewDefaultSystemChecker constructs a DefaultSystemChecker.
func NewDefaultSystemChecker(dockerCli DockerPinger) *DefaultSystemChecker {
	return &DefaultSystemChecker{dockerCli: dockerCli}
}

func (s *DefaultSystemChecker) CheckDiskSpace(ctx context.Context, path string) (int64, int64, error) {
	if path == "" {
		path = "/"
	}
	usage, err := disk.UsageWithContext(ctx, path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get disk usage: %w", err)
	}
	return int64(usage.Free), int64(usage.Total), nil
}

func (s *DefaultSystemChecker) CheckMemory(ctx context.Context) (int64, int64, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get memory info: %w", err)
	}
	return int64(vm.Available), int64(vm.Total), nil
}

func (s *DefaultSystemChecker) CheckDockerHealth(ctx context.Context) error {
	if s.dockerCli == nil {
		return fmt.Errorf("docker client is nil")
	}
	return s.dockerCli.CheckConnectivity(ctx)
}

// Checker validates host disk, memory, and Docker availability before heavy operations.
type Checker struct {
	sys        SystemResourceChecker
	thresholds Thresholds
	log        *slog.Logger
}

// NewChecker constructs a Checker.
func NewChecker(sys SystemResourceChecker, thresholds Thresholds, log *slog.Logger) *Checker {
	if log == nil {
		log = slog.Default()
	}
	defaults := DefaultThresholds()
	if thresholds.MinDiskFreeBytes <= 0 {
		thresholds.MinDiskFreeBytes = defaults.MinDiskFreeBytes
	}
	if thresholds.MinDiskFreePercent <= 0 {
		thresholds.MinDiskFreePercent = defaults.MinDiskFreePercent
	}
	if thresholds.MinMemoryFreeBytes <= 0 {
		thresholds.MinMemoryFreeBytes = defaults.MinMemoryFreeBytes
	}
	if thresholds.CheckPath == "" {
		thresholds.CheckPath = defaults.CheckPath
	}

	return &Checker{
		sys:        sys,
		thresholds: thresholds,
		log:        log,
	}
}

// ValidateExpensiveOperation performs all pre-flight safety checks.
// If any check fails, returns a typed *SafetyError containing one of:
// - CodeDockerUnavailable
// - CodeInsufficientDisk
// - CodeInsufficientMemory
func (c *Checker) ValidateExpensiveOperation(ctx context.Context) error {
	// 1. Docker Daemon Health
	if err := c.sys.CheckDockerHealth(ctx); err != nil {
		c.log.Error("pre-flight safety check failed: docker daemon unavailable", slog.String("error", err.Error()))
		return &SafetyError{
			Code:    CodeDockerUnavailable,
			Message: fmt.Sprintf("Docker daemon is unavailable or unhealthy: %s", err.Error()),
		}
	}

	// 2. Available Disk Space
	freeDisk, totalDisk, err := c.sys.CheckDiskSpace(ctx, c.thresholds.CheckPath)
	if err != nil {
		c.log.Warn("could not inspect host disk space, proceeding with caution", slog.String("error", err.Error()))
	} else {
		percentFree := float64(0)
		if totalDisk > 0 {
			percentFree = (float64(freeDisk) / float64(totalDisk)) * 100.0
		}

		if freeDisk < c.thresholds.MinDiskFreeBytes || percentFree < c.thresholds.MinDiskFreePercent {
			c.log.Error("pre-flight safety check failed: insufficient disk space",
				slog.Int64("free_bytes", freeDisk),
				slog.Int64("required_bytes", c.thresholds.MinDiskFreeBytes),
				slog.Float64("percent_free", percentFree),
			)
			return &SafetyError{
				Code:           CodeInsufficientDisk,
				Message:        fmt.Sprintf("Host disk space is below safety threshold (%.2f%% free)", percentFree),
				AvailableBytes: freeDisk,
				RequiredBytes:  c.thresholds.MinDiskFreeBytes,
			}
		}
	}

	// 3. Available Memory
	availMem, _, err := c.sys.CheckMemory(ctx)
	if err != nil {
		c.log.Warn("could not inspect host memory, proceeding with caution", slog.String("error", err.Error()))
	} else {
		if availMem < c.thresholds.MinMemoryFreeBytes {
			c.log.Error("pre-flight safety check failed: insufficient memory",
				slog.Int64("avail_bytes", availMem),
				slog.Int64("required_bytes", c.thresholds.MinMemoryFreeBytes),
			)
			return &SafetyError{
				Code:           CodeInsufficientMemory,
				Message:        "Host available memory is below safety threshold",
				AvailableBytes: availMem,
				RequiredBytes:  c.thresholds.MinMemoryFreeBytes,
			}
		}
	}

	return nil
}
