package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/deploycore/deploy-core/apps/agent/internal/agent"
)

func main() {
	// Early check for log level to setup the correct structured logger
	logLevel := os.Getenv("AGENT_LOG_LEVEL")
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	// In the future this might check if command is 'run', 'version', etc.
	// For A1, just initialize and run the agent.

	a, err := agent.New(logger)
	if err != nil {
		logger.Error("failed to initialize agent", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx := context.Background()
	if err := a.Run(ctx); err != nil {
		logger.Error("agent run failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("agent exited cleanly")
}
