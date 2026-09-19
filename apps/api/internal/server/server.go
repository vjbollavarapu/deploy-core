package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/applications"
	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/backups"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/databases"
	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/domains"
	"github.com/deploycore/deploy-core/apps/api/internal/gitproviders"
	"github.com/deploycore/deploy-core/apps/api/internal/health"
	"github.com/deploycore/deploy-core/apps/api/internal/healthchecks"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/logs"
	"github.com/deploycore/deploy-core/apps/api/internal/metrics"
	"github.com/deploycore/deploy-core/apps/api/internal/notifications"
	"github.com/deploycore/deploy-core/apps/api/internal/openapi"
	"github.com/deploycore/deploy-core/apps/api/internal/orchestrator"
	"github.com/deploycore/deploy-core/apps/api/internal/organizations"
	"github.com/deploycore/deploy-core/apps/api/internal/projects"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/registries"
	"github.com/deploycore/deploy-core/apps/api/internal/reconcile"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/deploycore/deploy-core/apps/api/internal/revisions"
	"github.com/deploycore/deploy-core/apps/api/internal/secrets"
	"github.com/deploycore/deploy-core/apps/api/internal/security"
	"github.com/deploycore/deploy-core/apps/api/internal/servers"
	"github.com/deploycore/deploy-core/apps/api/internal/variables"
	"github.com/deploycore/deploy-core/apps/api/internal/volumes"
	"github.com/deploycore/deploy-core/apps/api/internal/webhooks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server owns the HTTP listener and dependency wiring for the control plane.
type Server struct {
	cfg       config.Config
	log       *slog.Logger
	pool      *pgxpool.Pool
	http      *http.Server
	auth      *auth.Handler
	worker    *jobs.Worker
	queue     *jobs.Queue
	scheduler *reconcile.Scheduler
}

func New(cfg config.Config, log *slog.Logger, pool *pgxpool.Pool) *Server {
	mux := http.NewServeMux()
	healthHandler := health.NewHandler(pool)

	mux.HandleFunc("GET /health", healthHandler.Live)
	mux.HandleFunc("GET /ready", healthHandler.Ready)
	mux.Handle("GET /openapi.json", openapi.Handler())

	api := http.NewServeMux()
	api.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"deploycore-api","version":"v1"}`))
	})

	var authHandler *auth.Handler
	var jobWorker *jobs.Worker
	var jobQueue *jobs.Queue
	var reconcileScheduler *reconcile.Scheduler
	if pool != nil {
		authRepo := auth.NewPostgresRepository(pool)
		notifier := auth.PasswordResetNotifier(auth.NopPasswordResetNotifier{})
		if cfg.Env == "development" || cfg.Env == "test" {
			notifier = auth.LogPasswordResetNotifier{Log: log}
		}
		authSvc := auth.NewService(authRepo, auth.ServiceConfig{
			AccessTokenSecret: []byte(cfg.AuthTokenSecret),
			AccessTokenTTL:    cfg.AccessTokenTTL,
			RefreshTokenTTL:   cfg.RefreshTokenTTL,
			PasswordResetTTL:  cfg.PasswordResetTTL,
			MinPasswordLength: cfg.AuthMinPasswordLength,
		}, log, notifier)

		authz := rbac.NewAuthorizer(pool)
		auditWriter := audit.NewWriter(pool)
		authSvc.WithAudit(auditWriter)
		authHandler = auth.NewHandler(authSvc, auth.NewRateLimiter(cfg.AuthRateLimitPerMin, time.Minute))
		authHandler.Mount(api)

		auditRepo := audit.NewPostgresRepository(pool)
		auditSvc := audit.NewService(auditRepo, authz, log)
		audit.NewHandler(auditSvc, authHandler).Mount(api)

		orgRepo := organizations.NewPostgresRepository(pool)
		var inviteNotifier organizations.InvitationNotifier = organizations.NopInvitationNotifier{}
		if cfg.Env == "development" || cfg.Env == "test" {
			inviteNotifier = organizations.LogInvitationNotifier{Log: log}
		}
		orgSvc := organizations.NewService(orgRepo, authz, auditWriter, log, inviteNotifier)
		organizations.NewHandler(orgSvc, authHandler).Mount(api)

		projectRepo := projects.NewPostgresRepository(pool)
		projectSvc := projects.NewService(projectRepo, authz, auditWriter, log)
		projects.NewHandler(projectSvc, authHandler).Mount(api)

		serverRepo := servers.NewPostgresRepository(pool)
		serverSvc := servers.NewService(serverRepo, authz, auditWriter, log)
		servers.NewHandler(serverSvc, authHandler).Mount(api)

		agentRepo := agents.NewPostgresRepository(pool)
		agentSvc := agents.NewService(agentRepo, authz, auditWriter, log, agents.ServiceConfig{
			RegistrationTokenTTL: cfg.AgentRegistrationTTL,
			HeartbeatRetainCount: cfg.AgentHeartbeatRetain,
		})
		agentHandler := agents.NewHandler(agentSvc, authHandler, security.NewRateLimiter(cfg.PublicRateLimitPerMin, time.Minute))
		agentHandler.Mount(api)

		cmdRepo := agentcmd.NewPostgresRepository(pool)
		cmdSvc := agentcmd.NewService(cmdRepo, authz, auditWriter, log, agentcmd.ServiceConfig{})
		agentcmd.NewHandler(cmdSvc, authHandler, agentHandler.RequireAgent).Mount(api)

		appRepo := applications.NewPostgresRepository(pool)
		appSvc := applications.NewService(appRepo, authz, auditWriter, log).WithCapacity(serverSvc)
		applications.NewHandler(appSvc, authHandler).Mount(api)

		domainRepo := domains.NewPostgresRepository(pool)
		domainSvc := domains.NewService(domainRepo, authz, auditWriter, log)
		domains.NewHandler(domainSvc, authHandler).Mount(api)

		healthCheckRepo := healthchecks.NewPostgresRepository(pool)
		healthCheckSvc := healthchecks.NewService(healthCheckRepo, authz, log)
		healthchecks.NewHandler(healthCheckSvc, authHandler).Mount(api)

		logStore := logs.NewMemoryStore(logs.Config{})
		logAccess := logs.NewPostgresAccessor(pool)
		logSvc := logs.NewService(logStore, logAccess, authz, log, logs.Config{})
		logs.NewHandler(logSvc, authHandler, agentHandler.RequireAgent).Mount(api)

		metricRepo := metrics.NewPostgresRepository(pool)
		metricTS := metrics.NewMemoryTimeSeries(512)
		metricSvc := metrics.NewService(metricRepo, metricTS, authz, log)
		metrics.NewHandler(metricSvc, authHandler, agentHandler.RequireAgent).Mount(api)
		agentSvc.WithMetrics(metricSvc)

		dbRepo := databases.NewPostgresRepository(pool)
		dbSvc := databases.NewService(dbRepo, cmdRepo, authz, auditWriter, log, databases.ServiceConfig{
			PlatformKey: cfg.SecretsPlatformKey,
			KeyID:       cfg.SecretsKeyID,
		})
		databases.NewHandler(dbSvc, authHandler, agentHandler.RequireAgent).Mount(api)
		cmdSvc.WithCompletionHook(dbSvc.HandleCommandCompletion)

		volumeRepo := volumes.NewPostgresRepository(pool)
		volumeSvc := volumes.NewService(volumeRepo, cmdRepo, authz, auditWriter, log)
		volumes.NewHandler(volumeSvc, authHandler).Mount(api)
		cmdSvc.WithCompletionHook(volumeSvc.HandleCommandCompletion)
		dbSvc.WithVolumeRegistrar(volumeEnsure{svc: volumeSvc})

		jobRepo := jobs.NewPostgresRepository(pool)
		jobQueue = jobs.NewQueue(jobRepo, log)

		replicaRepo := replicas.NewPostgresRepository(pool)
		replicaSvc := replicas.NewService(replicaRepo, authz, auditWriter, jobQueue, log).WithCapacity(serverSvc)
		replicas.NewHandler(replicaSvc, authHandler).Mount(api)
		replicaReconciler := replicas.NewReconciler(pool, replicaRepo, log, cfg.OrchestratorSimulateAgent)
		reconcileLoop := reconcile.NewLoop(pool, agentSvc, replicaRepo, replicaReconciler, log, reconcile.Config{
			Enabled:                  cfg.ReconcileEnabled,
			Interval:                 cfg.ReconcileInterval,
			HeartbeatTTL:             cfg.AgentHeartbeatTTL,
			MaxAppActionsPerTick:     cfg.ReconcileMaxAppActions,
			MaxRestartActionsPerTick: cfg.ReconcileMaxRestartActions,
			RestartMaxAttempts:       cfg.ReconcileRestartMaxAttempts,
			RestartBackoffBase:       cfg.ReconcileRestartBackoffBase,
			SimulateAgent:            cfg.OrchestratorSimulateAgent,
		})
		if cfg.ReconcileEnabled {
			reconcileScheduler = reconcile.NewScheduler(jobQueue, reconcile.Config{
				Enabled:  true,
				Interval: cfg.ReconcileInterval,
			}, log)
		}

		backupRepo := backups.NewPostgresRepository(pool)
		backupSvc := backups.NewService(backupRepo, cmdRepo, jobQueue, authz, auditWriter, log)
		backups.NewHandler(backupSvc, authHandler).Mount(api)
		cmdSvc.WithCompletionHook(backupSvc.HandleCommandCompletion)

		notifyRepo := notifications.NewPostgresRepository(pool)
		notifySvc := notifications.NewService(notifyRepo, jobQueue, authz, auditWriter, log, notifications.ServiceConfig{
			PlatformKey: cfg.SecretsPlatformKey,
			KeyID:       cfg.SecretsKeyID,
		})
		notifications.NewHandler(notifySvc, authHandler).Mount(api)
		backupSvc.WithNotifier(notifySvc)
		agentSvc.WithNotifier(notifySvc)

		webhookRepo := webhooks.NewPostgresRepository(pool)
		webhookSvc := webhooks.NewService(webhookRepo, jobQueue, authz, auditWriter, log, webhooks.ServiceConfig{
			PlatformKey: cfg.SecretsPlatformKey,
			KeyID:       cfg.SecretsKeyID,
		})
		webhooks.NewHandler(webhookSvc, authHandler).Mount(api)
		backupSvc.WithWebhooks(webhookSvc)
		agentSvc.WithWebhooks(webhookSvc)

		varRepo := variables.NewPostgresRepository(pool)
		varSvc := variables.NewService(varRepo, authz, auditWriter, log)
		variables.NewHandler(varSvc, authHandler).Mount(api)

		secretRepo := secrets.NewPostgresRepository(pool)
		secretSvc := secrets.NewService(secretRepo, authz, auditWriter, log, secrets.ServiceConfig{
			PlatformKey: cfg.SecretsPlatformKey,
			KeyID:       cfg.SecretsKeyID,
		}).WithPool(pool)
		secrets.NewHandler(secretSvc, authHandler).Mount(api)

		deployRepo := deployments.NewPostgresRepository(pool)
		deploySvc := deployments.NewService(deployRepo, authz, auditWriter, log).WithPlacer(serverSvc)
		deployments.NewHandler(deploySvc, authHandler).Mount(api)

		revRepo := revisions.NewPostgresRepository(pool)
		revSvc := revisions.NewService(revRepo, authz)
		revisions.NewHandler(revSvc, authHandler).Mount(api)

		gitRepo := gitproviders.NewPostgresRepository(pool)
		gitSvc := gitproviders.NewService(gitRepo, deployRepo, authz, auditWriter, log, gitproviders.ServiceConfig{
			PlatformKey: cfg.SecretsPlatformKey,
			KeyID:       cfg.SecretsKeyID,
		})
		gitproviders.NewHandler(gitSvc, authHandler, security.NewRateLimiter(cfg.GitWebhookRateLimitPerMin, time.Minute)).Mount(api)

		registryRepo := registries.NewPostgresRepository(pool)
		registrySvc := registries.NewService(registryRepo, authz, auditWriter, log, registries.ServiceConfig{
			PlatformKey: cfg.SecretsPlatformKey,
			KeyID:       cfg.SecretsKeyID,
		})
		registries.NewHandler(registrySvc, authHandler).Mount(api)

		orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{
			SimulateAgent: cfg.OrchestratorSimulateAgent,
		}).WithHealthGate(healthCheckSvc).WithNotifier(notifySvc).WithWebhooks(webhookSvc)
		registry := jobs.DefaultRegistry(log)
		registry.Register(jobs.TypeDeploymentExecution, orch.JobHandler())
		registry.Register(jobs.TypeBackup, backupSvc.ProcessBackupJob)
		registry.Register(jobs.TypeRestore, backupSvc.ProcessRestoreJob)
		registry.Register(jobs.TypeNotificationDelivery, notifySvc.ProcessDeliveryJob)
		registry.Register(jobs.TypeWebhookDelivery, webhookSvc.ProcessDeliveryJob)
		registry.Register(jobs.TypeReplicasReconcile, replicaReconciler.JobHandler())
		registry.Register(jobs.TypeDesiredStateReconcile, reconcileLoop.JobHandler())
		jobWorker = jobs.NewWorker(jobQueue, registry, jobs.WorkerConfig{
			WorkerID:       cfg.JobWorkerID,
			LeaseTTL:       cfg.JobLeaseTTL,
			PollInterval:   cfg.JobPollInterval,
			RetryBaseDelay: cfg.JobRetryBaseDelay,
			DeferDelay:     cfg.JobDeferDelay,
			Enabled:        cfg.JobWorkerEnabled,
		}, log)
	}

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", api))

	handler := chain(
		mux,
		recoverMiddleware(log),
		requestIDMiddleware,
		security.MaxBytesMiddleware(cfg.MaxRequestBodyBytes),
		secureHeadersMiddleware,
		corsMiddleware(cfg.CORSAllowedOrigins),
		loggingMiddleware(log),
	)

	return &Server{
		cfg:       cfg,
		log:       log,
		pool:      pool,
		auth:      authHandler,
		worker:    jobWorker,
		queue:     jobQueue,
		scheduler: reconcileScheduler,
		http: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
	}
}

// StartWorkers starts the PostgreSQL job worker and desired-state reconcile scheduler.
func (s *Server) StartWorkers(ctx context.Context) {
	if s.worker != nil && s.cfg.JobWorkerEnabled {
		go s.worker.Run(ctx)
	}
	if s.scheduler != nil {
		go s.scheduler.Run(ctx)
	}
}

// JobQueue exposes the queue for tests and internal callers.
func (s *Server) JobQueue() *jobs.Queue {
	return s.queue
}

// Start begins listening. It blocks until the server stops.
func (s *Server) Start() error {
	s.log.Info("http server listening", slog.String("addr", s.cfg.HTTPAddr))
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// HTTPHandler exposes the configured handler for tests.
func (s *Server) HTTPHandler() http.Handler {
	return s.http.Handler
}

// IdleTimeout exposes configured shutdown budget for main.
func (s *Server) IdleTimeout() time.Duration {
	return s.cfg.ShutdownTimeout
}

type volumeEnsure struct {
	svc *volumes.Service
}

func (v volumeEnsure) EnsureDatabaseVolume(ctx context.Context, orgID, serverID uuid.UUID, name string, databaseID uuid.UUID, createdBy *uuid.UUID) error {
	_, err := v.svc.EnsureDatabaseVolume(ctx, orgID, serverID, name, databaseID, createdBy)
	return err
}
