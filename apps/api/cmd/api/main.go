package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.NewMigrator(pool).Up(ctx); err != nil {
		log.Error("migrations failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := rbac.EnsureSeeded(ctx, pool); err != nil {
		log.Error("rbac seed failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	srv := server.New(cfg, log, pool)
	srv.StartWorkers(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		log.Info("server stopped")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server exited", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
}
