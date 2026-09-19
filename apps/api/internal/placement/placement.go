package placement

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	ModeManual    = "manual"
	ModeScheduler = "scheduler"
)

// Policy is stored on applications.placement_policy.
type Policy struct {
	Mode          string            `json:"mode"`
	Labels        map[string]string `json:"labels"`
	Architecture  string            `json:"architecture"`
	AllowDegraded bool              `json:"allowDegraded"`
	Scoring       string            `json:"scoring"` // least_loaded (default) | most_free
}

// Request is a placement ask for an application workload.
type Request struct {
	CPUMillis    int
	MemoryBytes  int64
	DiskBytes    int64
	Policy       Policy
	ForcedServer *uuid.UUID // user-selected target
}

// Candidate is a schedulable server snapshot.
type Candidate struct {
	ID                   uuid.UUID
	Status               string
	MaintenanceMode      bool
	Labels               map[string]string
	Architecture         string
	CPUTotalMillis       int
	CPUAllocatedMillis   int
	MemoryTotalBytes     int64
	MemoryAllocatedBytes int64
	DiskTotalBytes       int64
	DiskAllocatedBytes   int64
}

// Result is the selected server (if any).
type Result struct {
	ServerID uuid.UUID
	Score    float64
	Reason   string
}

// ParsePolicy maps a JSON object into Policy with defaults.
func ParsePolicy(raw map[string]any) Policy {
	p := Policy{Mode: ModeManual, Scoring: "least_loaded"}
	if raw == nil {
		return p
	}
	if m, ok := raw["mode"].(string); ok {
		p.Mode = strings.ToLower(strings.TrimSpace(m))
	}
	if a, ok := raw["architecture"].(string); ok {
		p.Architecture = strings.TrimSpace(a)
	}
	if s, ok := raw["scoring"].(string); ok && strings.TrimSpace(s) != "" {
		p.Scoring = strings.ToLower(strings.TrimSpace(s))
	}
	if b, ok := raw["allowDegraded"].(bool); ok {
		p.AllowDegraded = b
	}
	if labels, ok := raw["labels"].(map[string]any); ok {
		p.Labels = map[string]string{}
		for k, v := range labels {
			if s, ok := v.(string); ok {
				p.Labels[k] = s
			}
		}
	}
	if labels, ok := raw["labels"].(map[string]string); ok {
		p.Labels = labels
	}
	if p.Mode == "" {
		p.Mode = ModeManual
	}
	return p
}

// Select picks a server. ForcedServer (manual) is preferred when set.
func Select(candidates []Candidate, req Request) (Result, error) {
	if req.ForcedServer != nil {
		for _, c := range candidates {
			if c.ID != *req.ForcedServer {
				continue
			}
			if err := validateForced(c, req); err != nil {
				return Result{}, fmt.Errorf("%w: %v", ErrInsufficientResources, err)
			}
			return Result{ServerID: c.ID, Score: 1, Reason: "user-selected server"}, nil
		}
		return Result{}, fmt.Errorf("%w: selected server not available", ErrInsufficientResources)
	}

	mode := req.Policy.Mode
	if mode != ModeScheduler {
		return Result{}, fmt.Errorf("%w: no target server and scheduler mode not enabled", ErrInsufficientResources)
	}

	var best *Candidate
	var bestScore float64
	var considered int
	for i := range candidates {
		c := candidates[i]
		if err := validateCandidate(c, req); err != nil {
			continue
		}
		considered++
		score := scoreCandidate(c, req)
		if best == nil || score > bestScore {
			cp := c
			best = &cp
			bestScore = score
		}
	}
	if best == nil {
		return Result{}, fmt.Errorf("%w: no candidate servers matched health, labels, and capacity (considered=%d)", ErrInsufficientResources, considered)
	}
	return Result{ServerID: best.ID, Score: bestScore, Reason: "scheduler"}, nil
}

// validateForced applies capacity/label checks for user-selected servers without requiring ONLINE.
func validateForced(c Candidate, req Request) error {
	if c.MaintenanceMode || c.Status == "MAINTENANCE" || c.Status == "DISABLED" {
		return fmt.Errorf("server in maintenance or disabled")
	}
	if arch := strings.TrimSpace(req.Policy.Architecture); arch != "" {
		if !strings.EqualFold(strings.TrimSpace(c.Architecture), arch) {
			return fmt.Errorf("architecture mismatch")
		}
	}
	for k, v := range req.Policy.Labels {
		if c.Labels[k] != v {
			return fmt.Errorf("label mismatch: %s", k)
		}
	}
	return validateCapacity(c, req)
}

func validateCandidate(c Candidate, req Request) error {
	if c.MaintenanceMode || c.Status == "MAINTENANCE" || c.Status == "DISABLED" {
		return fmt.Errorf("server in maintenance or disabled")
	}
	if c.Status == "OFFLINE" {
		return fmt.Errorf("server offline")
	}
	if c.Status == "DEGRADED" && !req.Policy.AllowDegraded {
		return fmt.Errorf("server degraded")
	}
	if c.Status != "ONLINE" && c.Status != "DEGRADED" {
		return fmt.Errorf("server unhealthy")
	}
	if arch := strings.TrimSpace(req.Policy.Architecture); arch != "" {
		if !strings.EqualFold(strings.TrimSpace(c.Architecture), arch) {
			return fmt.Errorf("architecture mismatch")
		}
	}
	for k, v := range req.Policy.Labels {
		if c.Labels[k] != v {
			return fmt.Errorf("label mismatch: %s", k)
		}
	}
	return validateCapacity(c, req)
}

func validateCapacity(c Candidate, req Request) error {
	if req.CPUMillis > 0 {
		if c.CPUTotalMillis <= 0 {
			return fmt.Errorf("server has no CPU capacity configured")
		}
		if c.CPUAllocatedMillis+req.CPUMillis > c.CPUTotalMillis {
			return fmt.Errorf("insufficient CPU")
		}
	}
	if req.MemoryBytes > 0 {
		if c.MemoryTotalBytes <= 0 {
			return fmt.Errorf("server has no memory capacity configured")
		}
		if c.MemoryAllocatedBytes+req.MemoryBytes > c.MemoryTotalBytes {
			return fmt.Errorf("insufficient memory")
		}
	}
	if req.DiskBytes > 0 {
		if c.DiskTotalBytes <= 0 {
			return fmt.Errorf("server has no disk capacity configured")
		}
		if c.DiskAllocatedBytes+req.DiskBytes > c.DiskTotalBytes {
			return fmt.Errorf("insufficient disk")
		}
	}
	return nil
}

func scoreCandidate(c Candidate, req Request) float64 {
	// Higher is better. Default: maximize free fraction across CPU+memory.
	var cpuFree, memFree float64
	if c.CPUTotalMillis > 0 {
		cpuFree = float64(c.CPUTotalMillis-c.CPUAllocatedMillis-req.CPUMillis) / float64(c.CPUTotalMillis)
	}
	if c.MemoryTotalBytes > 0 {
		memFree = float64(c.MemoryTotalBytes-c.MemoryAllocatedBytes-req.MemoryBytes) / float64(c.MemoryTotalBytes)
	}
	score := (cpuFree + memFree) / 2
	if req.Policy.Scoring == "most_free" {
		score = cpuFree*0.4 + memFree*0.6
	}
	if c.Status == "ONLINE" {
		score += 0.05
	}
	return score
}

// CoresToMillis converts inventory cores to millicores.
func CoresToMillis(cores *int) int {
	if cores == nil || *cores <= 0 {
		return 0
	}
	return *cores * 1000
}
