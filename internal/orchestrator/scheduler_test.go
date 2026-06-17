package orchestrator

import (
	"context"
	"testing"

	"github.com/local/symphony/internal/domain"
)

type testTracker struct {
	issues []domain.Issue
}

func (t testTracker) FetchIssues(context.Context, domain.ProjectConfig) ([]domain.Issue, error) {
	return t.issues, nil
}
func (testTracker) MarkStatus(context.Context, domain.Issue, domain.Status) error { return nil }

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
	project := domain.ProjectConfig{ID: "p", ActiveStates: []string{"open"}}
	cfg.Gitea.Projects = []domain.ProjectConfig{project}
	issue := domain.Issue{ProjectID: "p", ID: "1", Identifier: "acme/app#1", Title: "Do work", State: "open"}
	runner := &recordingRunner{}
	s := New(cfg, testTracker{issues: []domain.Issue{issue}})
	s.runner = runner

	s.poll(context.Background())
	s.wg.Wait()

	if len(runner.issues) != 1 || runner.issues[0].ID != "1" {
		t.Fatalf("runner issues = %#v, want issue 1", runner.issues)
	}
	if len(runner.projects) != 1 || runner.projects[0].ID != "p" {
		t.Fatalf("runner projects = %#v, want project p", runner.projects)
	}
}

func TestIsPendingDefaultsToOpenIssueWhenActiveStatesEmpty(t *testing.T) {
	var cfg domain.Config
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p"}}
	s := New(cfg, testTracker{})

	issue := domain.Issue{ProjectID: "p", ID: "1", State: "open"}
	if !s.isPending(issue) {
		t.Fatal("open issue with empty active_states is not pending")
	}
}

func TestIsPendingSkipsManagedStatusLabels(t *testing.T) {
	var cfg domain.Config
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p", ActiveStates: []string{"open"}}}
	s := New(cfg, testTracker{})

	for _, label := range []string{"symphony-running", "symphony-done", "symphony-failed"} {
		issue := domain.Issue{ProjectID: "p", ID: "1", State: "open", Labels: []string{label}}
		if s.isPending(issue) {
			t.Fatalf("issue with %s label is pending", label)
		}
	}
}
