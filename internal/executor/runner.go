package executor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/local/symphony/internal/domain"
	"github.com/local/symphony/internal/execution"
	"github.com/local/symphony/internal/reviewer"
	"github.com/local/symphony/internal/tracker"
	"github.com/local/symphony/internal/workspace"
)

// Runner processes one Task Issue through Agent Run, Review Gate, publish, and Done Handoff.
type Runner struct {
	Config  domain.Config
	Tracker tracker.Client
	Log     *slog.Logger

	createWorkspace func(context.Context, domain.Issue, domain.Config) (domain.Workspace, error)
	cleanWorkspace  func(context.Context, domain.Workspace) error
	createBranch    func(domain.Workspace, domain.Issue) error
	branchName      func(domain.Issue) string
	runCodex        func(context.Context, domain.Config, domain.Issue, domain.Workspace) error
	runReviewer     func(context.Context, string, time.Duration, string) error
	commitAndPush   func(context.Context, domain.Workspace, string, string) (domain.PublishResult, error)
}

// New creates a Runner. It panics if tracker is nil.
func New(cfg domain.Config, tr tracker.Client) *Runner {
	if tr == nil {
		panic("executor: tracker client is required")
	}
	return &Runner{
		Config:          cfg,
		Tracker:         tr,
		Log:             slog.Default(),
		createWorkspace: workspace.Create,
		cleanWorkspace:  workspace.Clean,
		createBranch:    execution.CreateBranch,
		branchName:      execution.BranchName,
		runCodex:        RunCodex,
		runReviewer:     reviewer.Run,
		commitAndPush:   execution.CommitAndPush,
	}
}

// Process runs the full Task Issue execution pipeline:
// mark running -> create workspace -> create branch -> Codex -> Review Gate -> publish -> mark done.
func (r *Runner) Process(ctx context.Context, issue domain.Issue, project domain.ProjectConfig) {
	log := r.logger().With(
		"project", project.ID,
		"issue", issue.Identifier,
		"title", issue.Title,
	)
	log.Info("processing issue")

	if err := r.Tracker.MarkStatus(ctx, issue, domain.StatusRunning); err != nil {
		log.Error("mark running failed", "error", err)
		return
	}

	failed := false
	var ws domain.Workspace
	fail := func(reason string) {
		failed = true
		if strings.TrimSpace(ws.Path) != "" {
			log.Error(reason, "workspace_path", ws.Path)
		} else {
			log.Error(reason)
		}
		_ = r.Tracker.MarkStatus(context.Background(), issue, domain.StatusFailed)
	}

	var err error
	ws, err = r.createWorkspace(ctx, issue, r.Config)
	if err != nil {
		fail(fmt.Sprintf("create workspace failed: %v", err))
		return
	}
	defer func() {
		if !shouldCleanWorkspace(failed, ws) {
			if failed && strings.TrimSpace(ws.Path) != "" {
				log.Info("preserving failed workspace", "workspace_path", ws.Path)
			}
			return
		}
		if cleanErr := r.cleanWorkspace(context.Background(), ws); cleanErr != nil {
			log.Error("clean workspace failed", "error", cleanErr)
		}
	}()

	if err := r.createBranch(ws, issue); err != nil {
		fail(fmt.Sprintf("create branch failed: %v", err))
		return
	}
	branch := r.branchName(issue)

	log.Info("running codex", "branch", branch)
	if err := r.runCodex(ctx, r.Config, issue, ws); err != nil {
		fail(fmt.Sprintf("codex failed: %v", err))
		return
	}

	log.Info("running reviewer")
	if err := r.runReviewer(ctx, r.Config.Reviewer.Command, r.Config.Reviewer.Timeout, ws.Path); err != nil {
		fail(fmt.Sprintf("reviewer failed: %v", err))
		return
	}

	log.Info("committing and pushing")
	result, err := r.commitAndPush(ctx, ws, branch, r.Config.Gitea.Token)
	if err != nil {
		fail(fmt.Sprintf("commit and push failed: %v", err))
		return
	}
	log.Info("pushed", "branch", result.Branch, "commit", result.Commit)

	if err := r.Tracker.MarkStatus(context.Background(), issue, domain.StatusDone); err != nil {
		log.Error("mark done failed", "error", err)
		return
	}
	log.Info("issue completed")
}

func (r *Runner) logger() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

func shouldCleanWorkspace(failed bool, ws domain.Workspace) bool {
	return !failed && strings.TrimSpace(ws.Path) != ""
}
