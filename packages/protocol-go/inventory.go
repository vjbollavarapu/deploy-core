package protocol

import (
	"errors"
	"strings"
	"time"
)

// HostInventory represents the complete static and hardware inventory of an agent host.
type HostInventory struct {
	Hostname         string    `json:"hostname"`
	OS               string    `json:"os"`
	Architecture     string    `json:"architecture"`
	KernelVersion    string    `json:"kernelVersion,omitempty"`
	CPUCores         int       `json:"cpuCores"`
	CPUModel         string    `json:"cpuModel,omitempty"`
	MemoryTotalBytes int64     `json:"memoryTotalBytes"`
	DiskTotalBytes   int64     `json:"diskTotalBytes"`
	DockerVersion    string    `json:"dockerVersion"`
	DockerAPIVersion string    `json:"dockerApiVersion,omitempty"`
	AgentVersion     string    `json:"agentVersion"`
	ProtocolMajor    int       `json:"protocolMajor"`
	ProtocolMinor    int       `json:"protocolMinor"`
	CollectedAt      time.Time `json:"collectedAt"`
}

// Validate checks that basic required host properties are populated.
func (h *HostInventory) Validate() error {
	if strings.TrimSpace(h.Hostname) == "" {
		return errors.New("hostname is required in host inventory")
	}
	if strings.TrimSpace(h.OS) == "" {
		return errors.New("os is required in host inventory")
	}
	if strings.TrimSpace(h.Architecture) == "" {
		return errors.New("architecture is required in host inventory")
	}
	if h.CPUCores <= 0 {
		return errors.New("cpuCores must be greater than 0")
	}
	if h.MemoryTotalBytes <= 0 {
		return errors.New("memoryTotalBytes must be greater than 0")
	}
	return nil
}
