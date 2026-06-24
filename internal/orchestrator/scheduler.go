package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/local/symphony/internal/domain"
	"github.com/local/symphony/internal/executor"
	"github.com/local/symphony/internal/tracker"
)

type issueRunner interface {
	Process(ctx context.Context, issue domain.Issue, project domain.ProjectConfig)
}

// Scheduler polls configured projects for pending issues and processes them.
type Scheduler struct {
	Config  domain.Config
	Tracker tracker.Client

	runner issueRunner
	sem    chan struct{}
	wg     sync.WaitGroup
}

// New creates a Scheduler with the given config and tracker client.
// It panics if tracker is nil.
func New(cfg domain.Config, tr tracker.Client) *Scheduler {
	if tr == nil {
		panic("orchestrator: tracker client is required")
	}
	return &Scheduler{
		Config:  cfg,
		Tracker: tr,
		runner:  executor.New(cfg, tr),
		sem:     make(chan struct{}, cfg.Scheduler.MaxConcurrent),
	}
}

// Run starts the poll loop. It blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	interval := s.Config.Scheduler.PollInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("scheduler started",
		"poll_interval", interval,
		"max_concurrent", cap(s.sem),
		"projects", len(s.Config.Gitea.Projects),
	)

	// Poll immediately on start.
	_ = s.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			slog.Info("scheduler stopped")
			return nil
		case <-ticker.C:
			_ = s.poll(ctx)
		}
	}
}

// RunOnce polls all projects once, waits for dispatched issues, and returns
// fetch errors so deployments can use it as a smoke check.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	slog.Info("scheduler run once started",
		"max_concurrent", cap(s.sem),
		"projects", len(s.Config.Gitea.Projects),
	)
	err := s.poll(ctx)
	s.wg.Wait()
	if err != nil {
		return err
	}
	slog.Info("scheduler run once completed")
	return nil
}

// poll fetches issues from all projects and dispatches pending ones.
func (s *Scheduler) poll(ctx context.Context) error {
	var fetchErr error
	failedProjects := 0
	for _, proj := range s.Config.Gitea.Projects {
		issues, err := s.Tracker.FetchPendingIssues(ctx, proj)
		if err != nil {
			slog.Error("fetch issues failed",
				"project", proj.ID,
				"error", err,
			)
			failedProjects++
			if fetchErr == nil {
				fetchErr = err
			}
			continue
		}

		dispatched := 0
		for _, issue := range issues {
			// Try to acquire a concurrency slot.
			select {
			case s.sem <- struct{}{}:
				s.wg.Add(1)
				go func(is domain.Issue, project domain.ProjectConfig) {
					defer s.wg.Done()
					defer func() { <-s.sem }()
					s.runner.Process(ctx, is, project)
				}(issue, proj)
				dispatched++
			default:
				// No slot available, skip remaining issues this round.
				slog.Debug("no concurrency slot available, skipping",
					"project", proj.ID,
				)
				break
			}
		}

		if dispatched > 0 {
			slog.Info("dispatched issues",
				"project", proj.ID,
				"count", dispatched,
			)
		}
	}
	if failedProjects > 0 {
		return fmt.Errorf("fetch issues failed for %d project(s): %w", failedProjects, fetchErr)
	}
	return nil
}
