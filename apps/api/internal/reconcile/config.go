package reconcile

import (
	"time"
)

// Config controls the desired-state reconciliation loop (B30).
type Config struct {
	// Interval between sweep job enqueues.
	Interval time.Duration
	// HeartbeatTTL marks ONLINE/DEGRADED servers OFFLINE when last heartbeat is older.
	HeartbeatTTL time.Duration
	// MaxAppActionsPerTick caps how many applications are reconciled per sweep.
	MaxAppActionsPerTick int
	// MaxRestartActionsPerTick caps restart-policy actions per sweep.
	MaxRestartActionsPerTick int
	// RestartMaxAttempts stops auto-restart after this many consecutive failures.
	RestartMaxAttempts int
	// RestartBackoffBase is multiplied by 2^attempt for next_restart_at.
	RestartBackoffBase time.Duration
	// SimulateAgent skips issuing agent commands (dev/test).
	SimulateAgent bool
	// Enabled toggles the scheduler ticker.
	Enabled bool
}

func (c Config) withDefaults() Config {
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
	if c.HeartbeatTTL <= 0 {
		c.HeartbeatTTL = 90 * time.Second
	}
	if c.MaxAppActionsPerTick <= 0 {
		c.MaxAppActionsPerTick = 50
	}
	if c.MaxRestartActionsPerTick <= 0 {
		c.MaxRestartActionsPerTick = 25
	}
	if c.RestartMaxAttempts <= 0 {
		c.RestartMaxAttempts = 5
	}
	if c.RestartBackoffBase <= 0 {
		c.RestartBackoffBase = 15 * time.Second
	}
	return c
}
