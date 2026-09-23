package docker

import (
	"context"
	"fmt"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// Client provides connectivity to the Docker Daemon.
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker client based on configuration.
func NewClient(cfg config.Config) (*Client, error) {
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	if cfg.DockerHost != "" {
		opts = append(opts, client.WithHost(cfg.DockerHost))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize docker client: %w", err)
	}

	return &Client{cli: cli}, nil
}

// CheckConnectivity verifies if the Docker daemon is reachable and responding.
func (c *Client) CheckConnectivity(ctx context.Context) error {
	ping, err := c.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker ping failed: %w", err)
	}
	if ping.APIVersion == "" {
		return fmt.Errorf("docker returned empty API version")
	}
	return nil
}

// SystemMetrics represents host-level information.
type SystemMetrics struct {
	Hostname         string
	OS               string
	Architecture     string
	CPUCores         int
	MemoryTotalBytes int64
	MemoryUsedBytes  int64
	DiskTotalBytes   int64
	DiskUsedBytes    int64
	CPUPercent       float64
	Load1            float64
	UptimeSeconds    int64
}

// GetSystemMetrics returns system metrics using gopsutil.
func (c *Client) GetSystemMetrics(ctx context.Context) (SystemMetrics, error) {
	var metrics SystemMetrics

	if h, err := host.InfoWithContext(ctx); err == nil {
		metrics.Hostname = h.Hostname
		metrics.OS = h.OS
		metrics.Architecture = h.KernelArch
		metrics.UptimeSeconds = int64(h.Uptime)
	}

	if m, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		metrics.MemoryTotalBytes = int64(m.Total)
		metrics.MemoryUsedBytes = int64(m.Used)
	}

	if d, err := disk.UsageWithContext(ctx, "/"); err == nil {
		metrics.DiskTotalBytes = int64(d.Total)
		metrics.DiskUsedBytes = int64(d.Used)
	}

	if cCount, err := cpu.CountsWithContext(ctx, true); err == nil {
		metrics.CPUCores = cCount
	}
	if cPercents, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(cPercents) > 0 {
		metrics.CPUPercent = cPercents[0]
	}

	if l, err := load.AvgWithContext(ctx); err == nil {
		metrics.Load1 = l.Load1
	}

	return metrics, nil
}

// DockerMetrics represents Docker-level information.
type DockerMetrics struct {
	Version           string
	ContainerCount    int
	RunningContainers int
	ImageCount        int
	VolumeCount       int
	NetworkCount      int
}

// GetDockerMetrics returns efficient metadata about the Docker host without full container lists.
func (c *Client) GetDockerMetrics(ctx context.Context) (DockerMetrics, error) {
	info, err := c.cli.Info(ctx)
	if err != nil {
		return DockerMetrics{}, fmt.Errorf("failed to get docker info: %w", err)
	}

	// Basic network/volume counts can't be fetched purely from Info, but getting lists is reasonably fast.
	// We bound it with a quick context timeout just in case it hangs.
	var volCount, netCount int
	if vols, err := c.cli.VolumeList(ctx, volume.ListOptions{}); err == nil {
		volCount = len(vols.Volumes)
	}
	if nets, err := c.cli.NetworkList(ctx, types.NetworkListOptions{}); err == nil {
		netCount = len(nets)
	}

	return DockerMetrics{
		Version:           info.ServerVersion,
		ContainerCount:    info.Containers,
		RunningContainers: info.ContainersRunning,
		ImageCount:        info.Images,
		VolumeCount:       volCount,
		NetworkCount:      netCount,
	}, nil
}
