package executor

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/local/symphony/internal/domain"
)

type recordingTracker struct {
	updates []domain.StatusUpdate
}

func (r *recordingTracker) FetchPendingIssues(context.Context, domain.ProjectConfig) ([]domain.Issue, error) {
	return nil, nil
}

func (r *recordingTracker) MarkStatus(_ context.Context, _ domain.Issue, update domain.StatusUpdate) error {
	r.updates = append(r.updates, update)
	return nil
}

func TestRunnerCompletesDoneHandoffInOrder(t *testing.T) {
	var cfg domain.Config
	cfg.Gitea.Token = "push-token"
	tr := &recordingTracker{}
	r := New(cfg, tr)

	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Title: "Fix login"}
	project := domain.ProjectConfig{ID: "p", RepoURL: "https://gitea.example.com/acme/app.git"}
	ws := domain.Workspace{Path: t.TempDir(), IssueKey: "p/issue-12-fix-login"}
	var calls []string
	cleaned := false

	r.createWorkspace = func(ctx context.Context, got domain.Issue, gotCfg domain.Config) (domain.Workspace, error) {
		calls = append(calls, "workspace")
		if !reflect.DeepEqual(got, issue) {
			t.Fatalf("createWorkspace issue = %#v, want %#v", got, issue)
		}
		return ws, nil
	}
	r.cleanWorkspace = func(ctx context.Context, got domain.Workspace) error {
		calls = append(calls, "clean")
		cleaned = true
		return nil
	}
	r.createBranch = func(got domain.Workspace, gotIssue domain.Issue) error {
		calls = append(calls, "branch")
		return nil
	}
	r.branchName = func(domain.Issue) string {
		return "symphony/p/issue-12-fix-login"
	}
	r.runCodex = func(ctx context.Context, gotCfg domain.Config, gotIssue domain.Issue, got domain.Workspace) error {
		calls = append(calls, "codex")
		return nil
	}
	r.runReviewer = func(ctx context.Context, command string, timeout time.Duration, path string) error {
		calls = append(calls, "reviewer")
		return nil
	}
	r.commitAndPush = func(ctx context.Context, got domain.Workspace, branch, token string) (domain.PublishResult, error) {
		calls = append(calls, "publish")
		if token != "push-token" {
			t.Fatalf("push token = %q, want push-token", token)
		}
		return domain.PublishResult{Branch: branch, Commit: "abc123"}, nil
	}

	r.Process(context.Background(), issue, project)

	if got := statuses(tr.updates); !reflect.DeepEqual(got, []domain.Status{domain.StatusRunning, domain.StatusDone}) {
		t.Fatalf("statuses = %#v", got)
	}
	if got := tr.updates[1].Publish; !reflect.DeepEqual(got, (domain.PublishResult{Branch: "symphony/p/issue-12-fix-login", Commit: "abc123"})) {
		t.Fatalf("publish = %#v", got)
	}
	wantCalls := []string{"workspace", "branch", "codex", "reviewer", "publish", "clean"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if !cleaned {
		t.Fatal("successful run did not clean workspace")
	}
}

func TestRunnerMarksFailedAndPreservesWorkspaceWhenReviewerFails(t *testing.T) {
	var cfg domain.Config
	tr := &recordingTracker{}
	r := New(cfg, tr)
	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Title: "Fix login"}
	project := domain.ProjectConfig{ID: "p"}
	ws := domain.Workspace{Path: t.TempDir(), IssueKey: "p/issue-12-fix-login"}
	cleaned := false
	published := false

	r.createWorkspace = func(context.Context, domain.Issue, domain.Config) (domain.Workspace, error) {
		return ws, nil
	}
	r.cleanWorkspace = func(context.Context, domain.Workspace) error {
		cleaned = true
		return nil
	}
	r.createBranch = func(domain.Workspace, domain.Issue) error { return nil }
	r.branchName = func(domain.Issue) string { return "symphony/p/issue-12-fix-login" }
	r.runCodex = func(context.Context, domain.Config, domain.Issue, domain.Workspace) error { return nil }
	r.runReviewer = func(context.Context, string, time.Duration, string) error {
		return errors.New("review failed")
	}
	r.commitAndPush = func(context.Context, domain.Workspace, string, string) (domain.PublishResult, error) {
		published = true
		return domain.PublishResult{}, nil
	}

	r.Process(context.Background(), issue, project)

	if got := statuses(tr.updates); !reflect.DeepEqual(got, []domain.Status{domain.StatusRunning, domain.StatusFailed}) {
		t.Fatalf("statuses = %#v", got)
	}
	failure := tr.updates[1]
	if !strings.Contains(failure.FailureReason, "reviewer failed: review failed") {
		t.Fatalf("failure reason = %q", failure.FailureReason)
	}
	if failure.WorkspacePath != ws.Path {
		t.Fatalf("failure workspace = %q, want %q", failure.WorkspacePath, ws.Path)
	}
	if cleaned {
		t.Fatal("failed run cleaned workspace; want preserved")
	}
	if published {
		t.Fatal("review failure published execution branch")
	}
}

func TestRunnerMarksFailedWithoutCleaningWhenWorkspaceCreateFails(t *testing.T) {
	var cfg domain.Config
	tr := &recordingTracker{}
	r := New(cfg, tr)
	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Title: "Fix login"}
	project := domain.ProjectConfig{ID: "p"}
	cleaned := false

	r.createWorkspace = func(context.Context, domain.Issue, domain.Config) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("clone failed")
	}
	r.cleanWorkspace = func(context.Context, domain.Workspace) error {
		cleaned = true
		return nil
	}

	r.Process(context.Background(), issue, project)

	if got := statuses(tr.updates); !reflect.DeepEqual(got, []domain.Status{domain.StatusRunning, domain.StatusFailed}) {
		t.Fatalf("statuses = %#v", got)
	}
	failure := tr.updates[1]
	if !strings.Contains(failure.FailureReason, "create workspace failed: clone failed") {
		t.Fatalf("failure reason = %q", failure.FailureReason)
	}
	if failure.WorkspacePath != "" {
		t.Fatalf("failure workspace = %q, want empty", failure.WorkspacePath)
	}
	if cleaned {
		t.Fatal("workspace create failure should not clean empty workspace")
	}
}

func statuses(updates []domain.StatusUpdate) []domain.Status {
	out := make([]domain.Status, 0, len(updates))
	for _, update := range updates {
		out = append(out, update.Status)
	}
	return out
}
