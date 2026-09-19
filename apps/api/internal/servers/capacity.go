package servers

import (
	"github.com/deploycore/deploy-core/apps/api/internal/placement"
)

func toCapacityView(s Server) CapacityView {
	cpuTotal := placement.CoresToMillis(s.CPUCores)
	memTotal := int64(0)
	if s.MemoryBytes != nil {
		memTotal = *s.MemoryBytes
	}
	diskTotal := int64(0)
	if s.DiskBytes != nil {
		diskTotal = *s.DiskBytes
	}
	return CapacityView{
		ServerID:             s.ID,
		Name:                 s.Name,
		Status:               s.Status,
		MaintenanceMode:      s.MaintenanceMode,
		Labels:               labelsOrEmpty(s.Labels),
		CPUTotalMillis:       cpuTotal,
		CPUAllocatedMillis:   s.CPUAllocatedMillis,
		CPUAvailableMillis:   maxInt(0, cpuTotal-s.CPUAllocatedMillis),
		MemoryTotalBytes:     memTotal,
		MemoryAllocatedBytes: s.MemoryAllocatedBytes,
		MemoryAvailableBytes: maxInt64(0, memTotal-s.MemoryAllocatedBytes),
		DiskTotalBytes:       diskTotal,
		DiskAllocatedBytes:   s.DiskAllocatedBytes,
		DiskAvailableBytes:   maxInt64(0, diskTotal-s.DiskAllocatedBytes),
	}
}

func toCandidate(s Server) placement.Candidate {
	memTotal := int64(0)
	if s.MemoryBytes != nil {
		memTotal = *s.MemoryBytes
	}
	diskTotal := int64(0)
	if s.DiskBytes != nil {
		diskTotal = *s.DiskBytes
	}
	arch := s.Architecture
	return placement.Candidate{
		ID:                   s.ID,
		Status:               s.Status,
		MaintenanceMode:      s.MaintenanceMode,
		Labels:               labelsOrEmpty(s.Labels),
		Architecture:         arch,
		CPUTotalMillis:       placement.CoresToMillis(s.CPUCores),
		CPUAllocatedMillis:   s.CPUAllocatedMillis,
		MemoryTotalBytes:     memTotal,
		MemoryAllocatedBytes: s.MemoryAllocatedBytes,
		DiskTotalBytes:       diskTotal,
		DiskAllocatedBytes:   s.DiskAllocatedBytes,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
