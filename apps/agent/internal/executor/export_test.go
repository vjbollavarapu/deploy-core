package executor

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
)

// Journal is an exported alias for the internal journal type to support testing.
type Journal = journal

// NewJournal creates a new journal for testing. If log is nil, a discard logger is used.
func NewJournal(dataDir string, log *slog.Logger) *Journal {
	if log == nil {
		log = slog.New(slog.NewTextHandler(noopWriter{}, nil))
	}
	return newJournal(dataDir, log)
}

// NewWithTransport creates an Executor with an injected transport for testing.
// A nil docker.Client means the executor will panic if a handler actually calls it —
// which is acceptable for tests that only exercise validation paths.
func NewWithTransport(cfg Config, t transport.Client) *Executor {
	log := slog.New(slog.NewTextHandler(noopWriter{}, nil))
	j := newJournal(cfg.DataDir, log)
	j.Load()

	concurrency := cfg.MaxConcurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	wsMgr := workspace.NewManager(filepath.Join(cfg.DataDir, "workspaces"))

	return &Executor{
		cfg:           cfg,
		transport:     t,
		handlers:      buildRegistry(nil, t, wsMgr, "", nil, log), // nil docker.Client — only validation-path tests
		journal:       j,
		sem:           make(chan struct{}, concurrency),
		activeCancels: make(map[string]context.CancelFunc),
		log:           log,
	}
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }
