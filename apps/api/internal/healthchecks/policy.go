package healthchecks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TypeHTTP      = "HTTP"
	TypeTCP       = "TCP"
	TypeCommand   = "COMMAND"
	TypeContainer = "CONTAINER"
)

const (
	StateUnknown   = "UNKNOWN"
	StateStarting  = "STARTING"
	StateHealthy   = "HEALTHY"
	StateDegraded  = "DEGRADED"
	StateUnhealthy = "UNHEALTHY"
)

// MaxSamplesRetained caps per-application probe history to avoid write amplification.
const MaxSamplesRetained = 20

// Policy is the structured health check configuration stored in application_configs.health_check.
type Policy struct {
	Type                string `json:"type"`
	Enabled             *bool  `json:"enabled,omitempty"`
	InitialDelaySeconds int    `json:"initialDelaySeconds"`
	IntervalSeconds     int    `json:"intervalSeconds"`
	TimeoutSeconds      int    `json:"timeoutSeconds"`
	Retries             int    `json:"retries"`
	Path                string `json:"path,omitempty"`
	Port                *int   `json:"port,omitempty"`
	ExpectedStatus      int    `json:"expectedStatus,omitempty"`
	Command             string `json:"command,omitempty"`
}

type Status struct {
	ApplicationID        uuid.UUID
	OrganizationID       uuid.UUID
	RevisionID           *uuid.UUID
	DeploymentID         *uuid.UUID
	State                string
	ConsecutiveSuccesses int
	ConsecutiveFailures  int
	LastProbeAt          *time.Time
	LastSuccessAt        *time.Time
	LastFailureAt        *time.Time
	LastMessage          string
	ProbeType            string
	UpdatedAt            time.Time
}

type Sample struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ApplicationID  uuid.UUID
	RevisionID     *uuid.UUID
	DeploymentID   *uuid.UUID
	Success        bool
	ProbeType      string
	Message        string
	LatencyMs      *int
	CreatedAt      time.Time
}

type ProbeInput struct {
	OrganizationID uuid.UUID
	ApplicationID  uuid.UUID
	RevisionID     *uuid.UUID
	DeploymentID   *uuid.UUID
	Success        bool
	ProbeType      string
	Message        string
	LatencyMs      *int
	Policy         Policy
}

type AuditMeta struct {
	IP        string
	UserAgent string
}

// ParsePolicy decodes health_check JSONB into a Policy with defaults applied.
func ParsePolicy(raw map[string]any) (Policy, error) {
	if raw == nil || len(raw) == 0 {
		p := DefaultPolicy()
		return p, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return Policy{}, fmt.Errorf("invalid health check object")
	}
	return Normalize(p)
}

func DefaultPolicy() Policy {
	enabled := true
	return Policy{
		Type:                TypeContainer,
		Enabled:             &enabled,
		InitialDelaySeconds: 0,
		IntervalSeconds:     10,
		TimeoutSeconds:      3,
		Retries:             3,
		Path:                "/healthz",
		ExpectedStatus:      200,
	}
}

func Normalize(p Policy) (Policy, error) {
	p.Type = strings.ToUpper(strings.TrimSpace(p.Type))
	if p.Type == "" {
		p.Type = TypeContainer
	}
	switch p.Type {
	case TypeHTTP, TypeTCP, TypeCommand, TypeContainer:
	default:
		return Policy{}, fmt.Errorf("type must be HTTP, TCP, COMMAND, or CONTAINER")
	}
	if p.Enabled == nil {
		t := true
		p.Enabled = &t
	}
	if p.InitialDelaySeconds < 0 {
		return Policy{}, fmt.Errorf("initialDelaySeconds must be >= 0")
	}
	if p.IntervalSeconds <= 0 {
		p.IntervalSeconds = 10
	}
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = 3
	}
	if p.Retries <= 0 {
		p.Retries = 3
	}
	if p.IntervalSeconds > 3600 || p.TimeoutSeconds > 600 {
		return Policy{}, fmt.Errorf("interval/timeout out of range")
	}
	if p.Retries > 20 {
		return Policy{}, fmt.Errorf("retries must be <= 20")
	}
	if p.Port != nil && (*p.Port <= 0 || *p.Port > 65535) {
		return Policy{}, fmt.Errorf("port must be 1-65535")
	}
	switch p.Type {
	case TypeHTTP:
		if strings.TrimSpace(p.Path) == "" {
			p.Path = "/healthz"
		}
		if !strings.HasPrefix(p.Path, "/") {
			return Policy{}, fmt.Errorf("path must start with /")
		}
		if p.ExpectedStatus == 0 {
			p.ExpectedStatus = 200
		}
		if p.ExpectedStatus < 100 || p.ExpectedStatus > 599 {
			return Policy{}, fmt.Errorf("expectedStatus must be a valid HTTP status")
		}
	case TypeTCP:
		if p.Port == nil {
			return Policy{}, fmt.Errorf("port is required for TCP health checks")
		}
	case TypeCommand:
		if strings.TrimSpace(p.Command) == "" {
			return Policy{}, fmt.Errorf("command is required for COMMAND health checks")
		}
	}
	return p, nil
}

func ValidateMap(raw map[string]any) error {
	_, err := ParsePolicy(raw)
	return err
}

func (p Policy) IsEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

func (p Policy) SuccessThreshold() int {
	if p.Retries <= 0 {
		return 1
	}
	return p.Retries
}

func (p Policy) FailureThreshold() int {
	return p.SuccessThreshold()
}

func (p Policy) ToAgentPayload() map[string]any {
	out := map[string]any{
		"type":                p.Type,
		"enabled":             p.IsEnabled(),
		"initialDelaySeconds": p.InitialDelaySeconds,
		"intervalSeconds":     p.IntervalSeconds,
		"timeoutSeconds":      p.TimeoutSeconds,
		"retries":             p.Retries,
	}
	if p.Path != "" {
		out["path"] = p.Path
	}
	if p.Port != nil {
		out["port"] = *p.Port
	}
	if p.ExpectedStatus != 0 {
		out["expectedStatus"] = p.ExpectedStatus
	}
	if p.Command != "" {
		out["command"] = p.Command
	}
	return out
}

// NextState computes aggregated health from consecutive counters.
func NextState(successes, failures, successThreshold, failureThreshold int, previously string) string {
	if successes >= successThreshold {
		return StateHealthy
	}
	if failures >= failureThreshold {
		return StateUnhealthy
	}
	if successes > 0 && failures > 0 {
		return StateDegraded
	}
	if previously == StateStarting || previously == "" || previously == StateUnknown {
		if successes > 0 || failures > 0 {
			return StateStarting
		}
		return StateUnknown
	}
	if successes > 0 {
		return StateStarting
	}
	if failures > 0 {
		return StateDegraded
	}
	return previously
}

// ReadyForActivation is true when policy is disabled or state reached HEALTHY.
func ReadyForActivation(p Policy, state string) bool {
	if !p.IsEnabled() {
		return true
	}
	return state == StateHealthy
}
