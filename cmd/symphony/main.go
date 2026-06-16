package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/local/symphony/internal/config"
	"github.com/local/symphony/internal/orchestrator"
	"github.com/local/symphony/internal/tracker/gitea"
)

func main() {
	configPath := flag.String("config", "symphony.yaml", "path to config file")
	flag.Parse()

	level := slog.LevelInfo
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	tracker := gitea.New(cfg.Gitea.Endpoint, cfg.Gitea.Token, cfg.Gitea.Projects, nil)

	sched := orchestrator.New(cfg, tracker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	slog.Info("symphony starting",
		"projects", len(cfg.Gitea.Projects),
		"poll_interval", cfg.Scheduler.PollInterval,
		"max_concurrent", cfg.Scheduler.MaxConcurrent,
	)

	if err := sched.Run(ctx); err != nil {
		slog.Error("scheduler exited", "error", err)
		os.Exit(1)
	}
}
