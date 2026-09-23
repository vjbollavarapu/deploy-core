package protocol

import (
	"errors"
	"fmt"
)

// ProtocolMajor indicates breaking wire incompatibilities.
const ProtocolMajor = 1

// ProtocolMinor indicates backward-compatible additions.
const ProtocolMinor = 0

// Stable Protocol Error Codes
const (
	ErrUnsupportedProtocolVersion = "UNSUPPORTED_PROTOCOL_VERSION"
	ErrInvalidProtocolMessage     = "INVALID_PROTOCOL_MESSAGE"
)

var (
	// ErrMajorVersionMismatch is returned when an agent or control plane version is fundamentally incompatible.
	ErrMajorVersionMismatch = errors.New("unsupported protocol major version: wire compatibility broken")
)

// AgentVersionReport contains the agent's reported version and protocol capability.
type AgentVersionReport struct {
	ProtocolMajor int    `json:"protocolMajor"`
	ProtocolMinor int    `json:"protocolMinor"`
	AgentVersion  string `json:"agentVersion"`
	CommitSHA     string `json:"commitSha,omitempty"`
}

// IsCompatible checks if the given major version matches the current protocol major version.
func IsCompatible(agentMajor int) bool {
	return agentMajor == ProtocolMajor
}

// CheckCompatibility checks if an agent's reported protocol version can communicate with this component.
func CheckCompatibility(agentMajor, agentMinor int) error {
	if agentMajor != ProtocolMajor {
		return fmt.Errorf("%w (expected major %d, got %d)", ErrMajorVersionMismatch, ProtocolMajor, agentMajor)
	}
	return nil
}
