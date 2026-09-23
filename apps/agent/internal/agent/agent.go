package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/controlplane"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/eventwatcher"
	"github.com/deploycore/deploy-core/apps/agent/internal/executor"
	"github.com/deploycore/deploy-core/apps/agent/internal/heartbeat"
	"github.com/deploycore/deploy-core/apps/agent/internal/registration"
	"github.com/deploycore/deploy-core/apps/agent/internal/runtime"
	"github.com/deploycore/deploy-core/apps/agent/internal/stats"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/apps/agent/pkg/version"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

type State string

const (
	StateStarting State = "STARTING"
	StateHealthy  State = "HEALTHY"
	StateDegraded State = "DEGRADED"
	StateError    State = "ERROR"
)

// Agent represents the main execution plane process.
type Agent struct {
	log          *slog.Logger
	cfg          config.Config
	paths        runtime.Paths
	cpCli        *controlplane.Client
	docCli       *docker.Client
	regMgr       *registration.Manager
	transport    transport.Client
	hbSampler    *heartbeat.Sampler
	executor     *executor.Executor
	eventWatcher *eventwatcher.Watcher
	statsSampler *stats.Sampler
	state        State
}

// New initializes the Agent process.
func New(log *slog.Logger) (*Agent, error) {
	log.Info("agent starting", slog.Any("version", version.Get()))

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	paths, err := runtime.InitPaths(cfg)
	if err != nil {
		return nil, fmt.Errorf("runtime initialization error: %w", err)
	}
	if cfg.CredentialPath == "" {
		cfg.CredentialPath = paths.CredentialPath
	}
	log.Info("runtime paths initialized", slog.String("data_dir", paths.DataDir))

	cpCli := controlplane.NewClient(cfg)

	docCli, err := docker.NewClient(cfg)
	if err != nil {
		// Log the error but don't fail startup completely just yet,
		// we will evaluate it during Run().
		log.Warn("docker client initialization failed", slog.String("error", err.Error()))
	}

	var docProvider heartbeat.DockerProvider
	if docCli != nil {
		docProvider = docCli
	}
	hbSampler := heartbeat.NewSampler(nil, docProvider)

	a := &Agent{
		log:       log,
		cfg:       cfg,
		paths:     paths,
		cpCli:     cpCli,
		docCli:    docCli,
		hbSampler: hbSampler,
		state:     StateStarting,
	}
	a.regMgr = registration.NewManager(&a.cfg, cpCli)

	// If ServerID is not set in environment, check if durable credentials already exist on disk
	if !a.cfg.IsRegistered() {
		if cred, err := a.regMgr.LoadCredential(); err == nil && cred.ServerID != "" {
			if sID, err := uuid.Parse(cred.ServerID); err == nil {
				a.cfg.ServerID = sID
				log.Info("agent loaded existing durable credentials", slog.String("server_id", sID.String()))
			}
		}
	}

	if !a.cfg.IsRegistered() {
		log.Info("agent starting in UNREGISTERED state")
	} else {
		log.Info("agent starting in REGISTERED state", slog.String("server_id", a.cfg.ServerID.String()))
	}

	return a, nil
}

// Stop gracefully stops the agent.
func (a *Agent) Stop(ctx context.Context) error {
	a.log.Info("stopping agent")

	// Drain in-flight commands before disconnecting transport
	if a.executor != nil {
		a.log.Info("waiting for in-flight commands to complete")
		a.executor.Stop()
		a.log.Info("executor drained")
	}

	if a.statsSampler != nil {
		a.statsSampler.Stop()
		a.log.Info("stats sampler stopped")
	}

	if a.eventWatcher != nil {
		a.eventWatcher.Stop()
		a.log.Info("docker event watcher stopped")
	}

	if a.transport != nil {
		if err := a.transport.Disconnect(ctx); err != nil {
			a.log.Error("failed to disconnect transport", slog.String("error", err.Error()))
		} else {
			a.log.Info("transport disconnected")
		}
	}

	return nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	interval := a.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Fire immediately once
	a.sendHeartbeat(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendHeartbeat(ctx)
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) {
	if a.transport == nil || a.hbSampler == nil {
		return
	}

	transportConnected := a.transport.CurrentState() == transport.StateConnected
	hb := a.hbSampler.Sample(ctx, transportConnected)
	a.state = State(hb.AgentState)

	if err := a.transport.SendHeartbeat(ctx, hb); err != nil {
		a.log.Error("failed to send heartbeat", slog.String("error", err.Error()))
		if a.state == StateHealthy {
			a.state = StateDegraded
		}
	}
}

// metricsPublishLoop flushes sampler snapshots to POST /agents/metrics (I10/R8).
func (a *Agent) metricsPublishLoop(ctx context.Context) {
	interval := a.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.publishMetrics(ctx)
		}
	}
}

func (a *Agent) publishMetrics(ctx context.Context) {
	if a.transport == nil || a.statsSampler == nil {
		return
	}
	if a.transport.CurrentState() != transport.StateConnected {
		return
	}
	latest := a.statsSampler.GetAllLatest()
	containers := make([]protocol.ContainerMetric, 0, len(latest))
	now := time.Now().UTC()
	for _, st := range latest {
		cpu := st.CPUPercent
		memUsed := st.MemoryUsage
		memLim := st.MemoryLimit
		rx := st.NetworkRx
		tx := st.NetworkTx
		restarts := st.RestartCount
		item := protocol.ContainerMetric{
			ContainerID:      st.ContainerID,
			ContainerName:    st.ContainerName,
			CPUPercent:       &cpu,
			MemoryUsedBytes:  &memUsed,
			MemoryLimitBytes: &memLim,
			NetworkRxBytes:   &rx,
			NetworkTxBytes:   &tx,
			RestartCount:     &restarts,
			Status:           st.Status,
			RecordedAt:       &now,
		}
		if st.ApplicationID != "" {
			appID := st.ApplicationID
			item.ApplicationID = &appID
		}
		containers = append(containers, item)
	}
	count := len(containers)
	req := protocol.MetricIngestRequest{
		Server: &protocol.ServerMetric{
			ContainerCount: &count,
			RecordedAt:     &now,
		},
		Containers: containers,
	}
	if err := a.transport.SendMetrics(ctx, req); err != nil {
		a.log.Warn("failed to send metrics", slog.String("error", err.Error()))
	}
}

// Run executes the agent lifecycle, blocking until a termination signal is received.
func (a *Agent) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a.log.Info("checking dependencies")

	// Perform registration if unregistered but token is present
	if !a.cfg.IsRegistered() && a.cfg.RegistrationToken != "" {
		a.log.Info("performing initial registration")
		regCtx, regCancel := context.WithTimeout(ctx, 30*time.Second)
		defer regCancel()

		if err := a.regMgr.PerformRegistration(regCtx); err != nil {
			a.log.Error("registration failed", slog.String("error", err.Error()))
			return fmt.Errorf("registration failed: %w", err)
		}

		a.log.Info("registration successful, credential saved securely")
	} else if !a.cfg.IsRegistered() {
		a.log.Warn("agent is unregistered and no registration token was provided. proceeding in degraded state.")
	}

	// If registered (either from startup or just now), load credentials and start transport
	if a.cfg.IsRegistered() {
		cred, err := a.regMgr.LoadCredential()
		if err != nil {
			a.log.Error("failed to load credentials", slog.String("error", err.Error()))
			return fmt.Errorf("failed to load credentials: %w", err)
		}

		a.transport = transport.NewHTTPClient(&a.cfg, cred)

		// Connect the transport
		if err := a.transport.Connect(ctx); err != nil {
			a.log.Error("failed to connect transport", slog.String("error", err.Error()))
		} else {
			a.log.Info("transport connected successfully")
		}

		// Monitor state in background
		go func() {
			for state := range a.transport.State() {
				a.log.Info("transport state changed", slog.String("state", string(state)))
				if state == transport.StateDisconnected {
					// Could transition Agent state here
					a.log.Warn("transport disconnected")
				}
			}
		}()

		// Start heartbeat loop
		go a.heartbeatLoop(ctx)

		// Start command execution framework
		a.executor = executor.New(executor.Config{
			ServerID:       a.cfg.ServerID,
			MaxConcurrency: executor.DefaultConcurrency,
			DataDir:        a.paths.DataDir,
		}, a.transport, a.docCli, a.paths, a.log)
		a.executor.Start(ctx)
		a.log.Info("command executor started")

		// Start Docker event watcher for managed containers (Phase A23)
		if a.docCli != nil {
			a.eventWatcher = eventwatcher.NewWatcher(a.docCli, func(ev eventwatcher.PlatformEvent) {
				a.log.Info("managed container event",
					slog.String("action", ev.Action),
					slog.String("container_id", ev.ContainerID),
					slog.String("container_name", ev.ContainerName),
					slog.String("app_id", ev.ApplicationID),
				)
				// Map Docker lifecycle events onto the Control Plane log ingest
				// contract (kind=runtime, stream=system). kind=event / stream=event
				// are rejected by CP validation and were silently dropped.
				if a.transport != nil && ev.ApplicationID != "" {
					var revID *string
					if ev.RevisionID != "" {
						rev := ev.RevisionID
						revID = &rev
					}
					_ = a.transport.SendLogs(ctx, protocol.LogIngestRequest{
						Kind:          "runtime",
						ApplicationID: ev.ApplicationID,
						RevisionID:    revID,
						Entries: []protocol.LogIngestLine{
							{
								Stream:    "system",
								Message:   fmt.Sprintf("[docker-event] action=%s container=%s id=%s", ev.Action, ev.ContainerName, ev.ContainerID),
								Timestamp: ev.Timestamp,
							},
						},
					})
				}
			}, a.log)
			a.eventWatcher.Start(ctx)
			a.log.Info("docker event watcher started")

			// Start container stats sampler (Phase A22) and continuous metrics transport (I10/R8).
			collector := stats.NewCollector(a.docCli)
			a.statsSampler = stats.NewSampler(collector, stats.DefaultSamplerConfig(), a.log)
			a.statsSampler.Start(ctx)
			a.log.Info("container stats sampler started")
			go a.metricsPublishLoop(ctx)
		}
	}

	// Perform health checks
	healthCtx, hbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer hbCancel()

	cpErr := a.cpCli.CheckConnectivity(healthCtx)
	if cpErr != nil {
		a.log.Warn("control plane connectivity degraded", slog.String("error", cpErr.Error()))
	} else {
		a.log.Info("control plane connectivity OK")
	}

	var docErr error
	if a.docCli != nil {
		docErr = a.docCli.CheckConnectivity(healthCtx)
		if docErr != nil {
			a.log.Warn("docker connectivity degraded", slog.String("error", docErr.Error()))
		} else {
			a.log.Info("docker connectivity OK")
		}
	} else {
		docErr = fmt.Errorf("docker client not initialized")
	}

	if cpErr == nil && docErr == nil {
		a.state = StateHealthy
	} else {
		a.state = StateDegraded
	}

	a.log.Info("agent ready", slog.String("state", string(a.state)))

	// Wait for shutdown signal
	<-ctx.Done()

	a.log.Info("shutdown signal received, initiating graceful shutdown")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	if err := a.Stop(stopCtx); err != nil {
		a.log.Error("graceful shutdown incomplete", slog.String("error", err.Error()))
		return err
	}
	a.log.Info("graceful shutdown complete")
	return nil
}
