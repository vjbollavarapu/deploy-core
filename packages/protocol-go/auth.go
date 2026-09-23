package protocol

import (
	"errors"
	"strings"
	"time"
)

// Standard Transport Headers
const (
	HeaderAgentToken      = "X-DeployCore-Agent-Token"
	HeaderServerID        = "X-DeployCore-Server-Id"
	HeaderProtocolVersion = "X-DeployCore-Protocol-Version"
	HeaderAgentVersion    = "X-DeployCore-Agent-Version"
)

// RegisterRequest is the payload sent by an agent to register itself.
type RegisterRequest struct {
	RegistrationToken string `json:"registrationToken"`
	AgentVersion      string `json:"agentVersion"`
	ProtocolMajor     int    `json:"protocolMajor,omitempty"`
	ProtocolMinor     int    `json:"protocolMinor,omitempty"`
	Hostname          string `json:"hostname,omitempty"`
	OS                string `json:"os,omitempty"`
	Architecture      string `json:"architecture,omitempty"`
}

// Validate checks that required registration fields are present.
func (r *RegisterRequest) Validate() error {
	if strings.TrimSpace(r.RegistrationToken) == "" {
		return errors.New("registration token is required")
	}
	if strings.TrimSpace(r.AgentVersion) == "" {
		return errors.New("agent version is required")
	}
	return nil
}

// RegisterResult is the response from a successful registration.
type RegisterResult struct {
	AgentID       string `json:"id"`
	ServerID      string `json:"serverId"`
	Credential    string `json:"credential"`
	TokenType     string `json:"tokenType"`
	ProtocolMajor int    `json:"protocolMajor,omitempty"`
	ProtocolMinor int    `json:"protocolMinor,omitempty"`
}

// Validate checks that required registration result fields are present.
func (r *RegisterResult) Validate() error {
	if strings.TrimSpace(r.ServerID) == "" {
		return errors.New("serverId is required in registration result")
	}
	if strings.TrimSpace(r.Credential) == "" {
		return errors.New("credential is required in registration result")
	}
	return nil
}

// AuthMetadata contains metadata for authenticating agent requests.
type AuthMetadata struct {
	ServerID      string `json:"serverId"`
	Token         string `json:"token"`
	ProtocolMajor int    `json:"protocolMajor"`
	AgentVersion  string `json:"agentVersion"`
}

// HeartbeatRequest is the payload sent by an agent to report its health and status.
type HeartbeatRequest struct {
	Timestamp       *time.Time `json:"timestamp"`
	AgentVersion    string     `json:"agentVersion"`
	ProtocolMajor   int        `json:"protocolMajor,omitempty"`
	ProtocolMinor   int        `json:"protocolMinor,omitempty"`
	DockerStatus    string     `json:"dockerStatus"`
	CPUPercent      *float64   `json:"cpuPercent"`
	MemoryUsedBytes *int64     `json:"memoryUsedBytes"`
	DiskUsedBytes   *int64     `json:"diskUsedBytes"`
	Load1           *float64   `json:"load1"`
	ContainerCount  *int       `json:"containerCount"`
	UptimeSeconds   *int64     `json:"uptimeSeconds"`
	DockerVersion   *string    `json:"dockerVersion"`

	Hostname         string `json:"hostname,omitempty"`
	OS               string `json:"os,omitempty"`
	Architecture     string `json:"architecture,omitempty"`
	CPUCores         int    `json:"cpuCores,omitempty"`
	MemoryTotalBytes int64  `json:"memoryTotalBytes,omitempty"`
	DiskTotalBytes   int64  `json:"diskTotalBytes,omitempty"`

	RunningContainers int    `json:"runningContainers,omitempty"`
	ImageCount        int    `json:"imageCount,omitempty"`
	VolumeCount       int    `json:"volumeCount,omitempty"`
	NetworkCount      int    `json:"networkCount,omitempty"`
	AgentState        string `json:"agentState,omitempty"`
}

// Validate checks that required heartbeat fields are present.
func (h *HeartbeatRequest) Validate() error {
	if strings.TrimSpace(h.AgentVersion) == "" {
		return errors.New("agentVersion is required in heartbeat")
	}
	if strings.TrimSpace(h.DockerStatus) == "" {
		return errors.New("dockerStatus is required in heartbeat")
	}
	return nil
}

// HeartbeatResponse is the response returned by the control plane to an agent heartbeat.
type HeartbeatResponse struct {
	Acknowledged        bool      `json:"acknowledged"`
	ServerTime          time.Time `json:"serverTime"`
	NextHeartbeatPeriod int       `json:"nextHeartbeatPeriodSeconds,omitempty"`
}
