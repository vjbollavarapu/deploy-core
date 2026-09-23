package heartbeat

import (
	"context"
	"os"
	goRuntime "runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/pkg/version"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemMetrics contains host-level inventory and resource consumption.
type SystemMetrics struct {
	Hostname         string
	OS               string
	Architecture     string
	UptimeSeconds    int64
	CPUCores         int
	CPUPercent       float64
	MemoryTotalBytes int64
	MemoryUsedBytes  int64
	DiskTotalBytes   int64
	DiskUsedBytes    int64
	Load1            float64
}

// SystemCollector collects host hardware and OS telemetry.
type SystemCollector interface {
	CollectSystemMetrics(ctx context.Context) (SystemMetrics, error)
}

// DefaultSystemCollector collects metrics using gopsutil and standard OS calls.
type DefaultSystemCollector struct{}

// CollectSystemMetrics gathers host telemetry.
func (d *DefaultSystemCollector) CollectSystemMetrics(ctx context.Context) (SystemMetrics, error) {
	var m SystemMetrics

	// Host identity
	if h, err := host.InfoWithContext(ctx); err == nil {
		m.Hostname = h.Hostname
		m.OS = h.OS
		m.Architecture = h.KernelArch
		m.UptimeSeconds = int64(h.Uptime)
	} else {
		// Fallbacks if gopsutil host info fails
		m.Hostname, _ = os.Hostname()
		m.OS = goRuntime.GOOS
		m.Architecture = goRuntime.GOARCH
	}

	// Memory telemetry
	if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		m.MemoryTotalBytes = int64(v.Total)
		m.MemoryUsedBytes = int64(v.Used)
	}

	// Disk telemetry for root partition
	if d, err := disk.UsageWithContext(ctx, "/"); err == nil {
		m.DiskTotalBytes = int64(d.Total)
		m.DiskUsedBytes = int64(d.Used)
	}

	// CPU telemetry
	if cCount, err := cpu.CountsWithContext(ctx, true); err == nil {
		m.CPUCores = cCount
	} else {
		m.CPUCores = goRuntime.NumCPU()
	}

	if cPercents, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(cPercents) > 0 {
		m.CPUPercent = cPercents[0]
	}

	// Load average (supported on Unix)
	if l, err := load.AvgWithContext(ctx); err == nil {
		m.Load1 = l.Load1
	}

	return m, nil
}

// DockerProvider defines the interface for retrieving Docker telemetry.
type DockerProvider interface {
	GetDockerMetrics(ctx context.Context) (docker.DockerMetrics, error)
}

// Sampler manages efficient metric collection and heartbeat request generation.
type Sampler struct {
	sysCollector SystemCollector
	docProvider  DockerProvider
	seq          uint64

	mu             sync.Mutex
	cachedVolCount int
	cachedNetCount int
	lastScan       time.Time
}

// NewSampler creates a new heartbeat sampler.
func NewSampler(sys SystemCollector, doc DockerProvider) *Sampler {
	if sys == nil {
		sys = &DefaultSystemCollector{}
	}
	return &Sampler{
		sysCollector: sys,
		docProvider:  doc,
	}
}

// NextSequence increments and returns the monotonically increasing sequence number.
func (s *Sampler) NextSequence() uint64 {
	return atomic.AddUint64(&s.seq, 1)
}

// Sequence returns the current sequence number.
func (s *Sampler) Sequence() uint64 {
	return atomic.LoadUint64(&s.seq)
}

// Sample gathers host and Docker metrics, evaluates state, and constructs a protocol.HeartbeatRequest.
func (s *Sampler) Sample(ctx context.Context, transportConnected bool) protocol.HeartbeatRequest {
	_ = s.NextSequence()
	now := time.Now().UTC()

	hb := protocol.HeartbeatRequest{
		Timestamp:     &now,
		AgentVersion:  version.Get().Version,
		ProtocolMajor: 1,
		ProtocolMinor: 0,
	}

	// 1. Gather host system inventory
	sys, err := s.sysCollector.CollectSystemMetrics(ctx)
	if err == nil {
		hb.Hostname = sys.Hostname
		hb.OS = sys.OS
		hb.Architecture = sys.Architecture
		hb.CPUCores = sys.CPUCores
		hb.MemoryTotalBytes = sys.MemoryTotalBytes
		hb.DiskTotalBytes = sys.DiskTotalBytes

		hb.CPUPercent = &sys.CPUPercent
		hb.MemoryUsedBytes = &sys.MemoryUsedBytes
		hb.DiskUsedBytes = &sys.DiskUsedBytes
		hb.Load1 = &sys.Load1
		hb.UptimeSeconds = &sys.UptimeSeconds
	} else {
		// Minimum fallback info
		h, _ := os.Hostname()
		hb.Hostname = h
		hb.OS = goRuntime.GOOS
		hb.Architecture = goRuntime.GOARCH
		hb.CPUCores = goRuntime.NumCPU()
	}

	// 2. Gather Docker Engine inventory
	dockerOK := false
	if s.docProvider != nil {
		doc, err := s.docProvider.GetDockerMetrics(ctx)
		if err == nil {
			dockerOK = true
			hb.DockerStatus = "ONLINE"
			hb.DockerVersion = &doc.Version
			hb.ContainerCount = &doc.ContainerCount
			hb.RunningContainers = doc.RunningContainers
			hb.ImageCount = doc.ImageCount

			// Efficient caching: update volume and network counts periodically rather than scanning every tick
			s.mu.Lock()
			if time.Since(s.lastScan) > 60*time.Second || s.cachedVolCount == 0 && s.cachedNetCount == 0 {
				s.cachedVolCount = doc.VolumeCount
				s.cachedNetCount = doc.NetworkCount
				s.lastScan = now
			}
			volCount := s.cachedVolCount
			netCount := s.cachedNetCount
			s.mu.Unlock()

			hb.VolumeCount = volCount
			hb.NetworkCount = netCount
		} else {
			hb.DockerStatus = "OFFLINE"
		}
	} else {
		hb.DockerStatus = "OFFLINE"
	}

	// 3. Derive agent health state
	switch {
	case !transportConnected && !dockerOK:
		hb.AgentState = "ERROR"
	case !transportConnected:
		hb.AgentState = "DEGRADED"
	case !dockerOK:
		hb.AgentState = "DEGRADED"
	default:
		hb.AgentState = "HEALTHY"
	}

	return hb
}
