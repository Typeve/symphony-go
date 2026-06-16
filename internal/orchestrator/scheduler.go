package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/local/symphony/internal/agentenv"
	"github.com/local/symphony/internal/domain"
	"github.com/local/symphony/internal/execution"
	"github.com/local/symphony/internal/reviewer"
	"github.com/local/symphony/internal/tracker"
	"github.com/local/symphony/internal/workspace"
)

// Scheduler polls configured projects for pending issues and processes them.
type Scheduler struct {
	Config  domain.Config
	Tracker tracker.Client

	sem chan struct{}
	wg  sync.WaitGroup
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
				go func(is domain.Issue) {
					defer s.wg.Done()
					defer func() { <-s.sem }()
					s.processIssue(ctx, is, proj)
				}(issue)
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

// processIssue runs the full pipeline for a single issue:
// mark running → create workspace → create branch → codex → review → commit & push → mark done.
func (s *Scheduler) processIssue(ctx context.Context, issue domain.Issue, proj domain.ProjectConfig) {
	log := slog.With(
		"project", proj.ID,
		"issue", issue.Identifier,
		"title", issue.Title,
	)
	log.Info("processing issue")

	// Mark as running.
	if err := s.Tracker.MarkStatus(ctx, issue, domain.StatusRunning); err != nil {
		log.Error("mark running failed", "error", err)
		return
	}

	fail := func(reason string) {
		log.Error(reason)
		_ = s.Tracker.MarkStatus(context.Background(), issue, domain.StatusFailed)
	}

	// Create workspace (clones repo).
	ws, err := workspace.Create(ctx, issue, s.Config)
	if err != nil {
		fail(fmt.Sprintf("create workspace failed: %v", err))
		return
	}
	defer func() {
		if cleanErr := workspace.Clean(context.Background(), ws); cleanErr != nil {
			log.Error("clean workspace failed", "error", cleanErr)
		}
	}()

	// Create branch.
	if err := execution.CreateBranch(ws, issue); err != nil {
		fail(fmt.Sprintf("create branch failed: %v", err))
		return
	}
	branch := execution.BranchName(issue)

	// Run Codex.
	log.Info("running codex", "branch", branch)
	if err := s.runCodex(ctx, issue, ws); err != nil {
		fail(fmt.Sprintf("codex failed: %v", err))
		return
	}

	// Run reviewer.
	log.Info("running reviewer")
	if err := reviewer.Run(ctx, s.Config.Reviewer.Command, s.Config.Reviewer.Timeout, ws.Path); err != nil {
		fail(fmt.Sprintf("reviewer failed: %v", err))
		return
	}

	// Commit and push.
	log.Info("committing and pushing")
	result, err := execution.CommitAndPush(ctx, ws, branch, s.Config.Gitea.Token)
	if err != nil {
		fail(fmt.Sprintf("commit and push failed: %v", err))
		return
	}
	log.Info("pushed", "branch", result.Branch, "commit", result.Commit)

	// Mark as done.
	if err := s.Tracker.MarkStatus(context.Background(), issue, domain.StatusDone); err != nil {
		log.Error("mark done failed", "error", err)
		return
	}
	log.Info("issue completed")
}

// runCodex executes the Codex CLI command in the workspace directory.
func (s *Scheduler) runCodex(ctx context.Context, issue domain.Issue, ws domain.Workspace) error {
	cmdStr := strings.TrimSpace(s.Config.Codex.Command)
	if cmdStr == "" {
		cmdStr = "codex"
	}

	timeout := s.Config.Codex.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prompt := s.buildCodexPrompt(issue)

	cmd := exec.CommandContext(ctx, cmdStr, "--prompt", prompt)
	cmd.Dir = ws.Path
	cmd.Env = agentenv.Filter(os.Environ())

	// Run without inheriting stdout/stderr to avoid noise.
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex: %w", err)
	}
	return nil
}

// buildCodexPrompt constructs a prompt string for the Codex agent.
func (s *Scheduler) buildCodexPrompt(issue domain.Issue) string {
	var b strings.Builder
	b.WriteString("Resolve the following issue:\n\n")
	b.WriteString(fmt.Sprintf("Title: %s\n", issue.Title))
	if issue.Description != nil && *issue.Description != "" {
		b.WriteString(fmt.Sprintf("Description:\n%s\n", *issue.Description))
	}
	b.WriteString("\nImplement the necessary changes in this repository.")
	return b.String()
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
