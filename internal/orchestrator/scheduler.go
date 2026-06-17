package orchestrator

import (
	"context"
	"log/slog"
	"strings"
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
	max := cfg.Scheduler.MaxConcurrent
	if max <= 0 {
		max = 1
	}
	return &Scheduler{
		Config:  cfg,
		Tracker: tr,
		runner:  executor.New(cfg, tr),
		sem:     make(chan struct{}, max),
	}
}

// Run starts the poll loop. It blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	interval := s.Config.Scheduler.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("scheduler started",
		"poll_interval", interval,
		"max_concurrent", cap(s.sem),
		"projects", len(s.Config.Gitea.Projects),
	)

	// Poll immediately on start.
	s.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			slog.Info("scheduler stopped")
			return nil
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll fetches issues from all projects and dispatches pending ones.
func (s *Scheduler) poll(ctx context.Context) {
	for _, proj := range s.Config.Gitea.Projects {
		issues, err := s.Tracker.FetchIssues(ctx, proj)
		if err != nil {
			slog.Error("fetch issues failed",
				"project", proj.ID,
				"error", err,
			)
			continue
		}

		dispatched := 0
		for _, issue := range issues {
			if !s.isPending(issue) {
				continue
			}
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
}

// isPending checks if an issue is in a state that should be processed.
func (s *Scheduler) isPending(issue domain.Issue) bool {
	proj, ok := s.findProject(issue)
	if !ok {
		return false
	}
	if hasManagedStatusLabel(issue.Labels) {
		return false
	}
	activeStates := proj.ActiveStates
	if len(activeStates) == 0 {
		activeStates = []string{"open"}
	}
	for _, active := range activeStates {
		if strings.EqualFold(strings.TrimSpace(issue.State), strings.TrimSpace(active)) {
			return true
		}
	}
	return false
}

// findProject returns the ProjectConfig for the given issue.
func (s *Scheduler) findProject(issue domain.Issue) (domain.ProjectConfig, bool) {
	for _, p := range s.Config.Gitea.Projects {
		if strings.EqualFold(strings.TrimSpace(p.ID), strings.TrimSpace(issue.ProjectID)) {
			return p, true
		}
	}
	return domain.ProjectConfig{}, false
}

func hasManagedStatusLabel(labels []string) bool {
	for _, label := range labels {
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "symphony-running", "symphony-done", "symphony-failed":
			return true
		}
	}
	return false
}
