package transport

import (
	"context"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type ConnectionState string

const (
	StateConnected    ConnectionState = "CONNECTED"
	StateDisconnected ConnectionState = "DISCONNECTED"
	StateReconnecting ConnectionState = "RECONNECTING"
)

// Client defines the contract for Control Plane communication.
type Client interface {
	// Connect establishes the connection (or starts polling).
	Connect(ctx context.Context) error

	// PollCommands blocks until commands are received or context is cancelled.
	PollCommands(ctx context.Context) ([]protocol.CommandEnvelope, error)

	// SendCommandStatus sends the execution status of a command back to the control plane.
	SendCommandStatus(ctx context.Context, cmdID string, status protocol.CommandStatusRequest) error

	// SendHeartbeat sends a ping/keepalive and telemetry data.
	SendHeartbeat(ctx context.Context, hb protocol.HeartbeatRequest) error

	// SendLogs streams log entries or deployment events to the control plane.
	SendLogs(ctx context.Context, req protocol.LogIngestRequest) error

	// SendMetrics reports server/container metric snapshots to the control plane.
	SendMetrics(ctx context.Context, req protocol.MetricIngestRequest) error

	// FetchDatabaseBootstrap retrieves provision/runtime secrets for a managed database.
	// Password must never be logged by callers.
	FetchDatabaseBootstrap(ctx context.Context, databaseID string) (protocol.DatabaseBootstrap, error)

	// State returns the current connection state channel for reporting.
	State() <-chan ConnectionState

	// CurrentState returns the current connection state.
	CurrentState() ConnectionState

	// Ping tests authenticated connectivity to the control plane.
	Ping(ctx context.Context) error

	// Disconnect gracefully shuts down the transport.
	Disconnect(ctx context.Context) error
}
