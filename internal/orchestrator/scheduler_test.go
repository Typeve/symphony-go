package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/local/symphony/internal/domain"
)

type testTracker struct {
	issues []domain.Issue
	err    error
}

func (t testTracker) FetchPendingIssues(context.Context, domain.ProjectConfig) ([]domain.Issue, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.issues, nil
}
func (testTracker) MarkStatus(context.Context, domain.Issue, domain.Status, ...domain.PublishResult) error {
	return nil
}

type recordingRunner struct {
	issues   []domain.Issue
	projects []domain.ProjectConfig
}

func (r *recordingRunner) Process(_ context.Context, issue domain.Issue, project domain.ProjectConfig) {
	r.issues = append(r.issues, issue)
	r.projects = append(r.projects, project)
}

func TestPollDispatchesPendingIssueToRunner(t *testing.T) {
	var cfg domain.Config
	cfg.Scheduler.MaxConcurrent = 1
	project := domain.ProjectConfig{ID: "p"}
	cfg.Gitea.Projects = []domain.ProjectConfig{project}
	issue := domain.Issue{ProjectID: "p", ID: "1", Identifier: "acme/app#1", Title: "Do work"}
	runner := &recordingRunner{}
	s := New(cfg, testTracker{issues: []domain.Issue{issue}})
	s.runner = runner

	if err := s.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}
	s.wg.Wait()

	if len(runner.issues) != 1 || runner.issues[0].ID != "1" {
		t.Fatalf("runner issues = %#v, want issue 1", runner.issues)
	}
	if len(runner.projects) != 1 || runner.projects[0].ID != "p" {
		t.Fatalf("runner projects = %#v, want project p", runner.projects)
	}
}

func TestPollDoesNotDispatchWhenFetchPendingIssuesFails(t *testing.T) {
	var cfg domain.Config
	cfg.Scheduler.MaxConcurrent = 1
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p"}}
	runner := &recordingRunner{}
	s := New(cfg, testTracker{err: errors.New("tracker unavailable")})
	s.runner = runner

	err := s.poll(context.Background())
	s.wg.Wait()

	if err == nil {
		t.Fatal("poll returned nil error, want fetch failure")
	}
	if len(runner.issues) != 0 {
		t.Fatalf("runner issues = %#v, want no dispatch", runner.issues)
	}
}

func TestRunOnceWaitsForDispatchedIssues(t *testing.T) {
	var cfg domain.Config
	cfg.Scheduler.MaxConcurrent = 1
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p"}}
	issue := domain.Issue{ProjectID: "p", ID: "1", Identifier: "acme/app#1", Title: "Do work"}
	runner := &recordingRunner{}
	s := New(cfg, testTracker{issues: []domain.Issue{issue}})
	s.runner = runner

	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if len(runner.issues) != 1 {
		t.Fatalf("runner issues = %#v, want dispatched issue", runner.issues)
	}
}

func TestRunOnceReturnsFetchError(t *testing.T) {
	var cfg domain.Config
	cfg.Scheduler.MaxConcurrent = 1
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p"}}
	s := New(cfg, testTracker{err: errors.New("tracker unavailable")})

	if err := s.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce returned nil error, want fetch failure")
	}
}

func TestPollDoesNotDispatchWhenConcurrencySlotUnavailable(t *testing.T) {
	var cfg domain.Config
	cfg.Scheduler.MaxConcurrent = 1
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p"}}
	issue := domain.Issue{ProjectID: "p", ID: "1", Identifier: "acme/app#1", Title: "Do work"}
	runner := &recordingRunner{}
	s := New(cfg, testTracker{issues: []domain.Issue{issue}})
	s.runner = runner

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	if err := s.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}
	s.wg.Wait()

	if len(runner.issues) != 0 {
		t.Fatalf("runner issues = %#v, want no dispatch", runner.issues)
	}
}
