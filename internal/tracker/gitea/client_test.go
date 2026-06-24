package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/local/symphony/internal/domain"
)

func TestFetchPendingIssuesSkipsManagedStatusLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/acme/app/issues" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("state"); got != "open" {
			t.Fatalf("state query = %q, want open", got)
		}
		_ = json.NewEncoder(w).Encode([]giteaIssue{
			{Number: 1, Title: "ready"},
			{Number: 2, Title: "running", Labels: []giteaLabel{{Name: "symphony-running"}}},
			{Number: 3, Title: "done", Labels: []giteaLabel{{Name: "symphony-done"}}},
			{Number: 4, Title: "failed", Labels: []giteaLabel{{Name: "symphony-failed"}}},
		})
	}))
	defer server.Close()

	project := domain.ProjectConfig{ID: "p", RepoURL: "https://gitea.example.com/acme/app.git"}
	client := New(server.URL, "token", []domain.ProjectConfig{project}, server.Client())

	issues, err := client.FetchPendingIssues(context.Background(), project)
	if err != nil {
		t.Fatalf("FetchPendingIssues returned error: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "1" {
		t.Fatalf("issues = %#v, want only issue 1", issues)
	}
}

func TestFetchPendingIssuesRejectsInvalidActiveStates(t *testing.T) {
	project := domain.ProjectConfig{
		ID:           "p",
		RepoURL:      "https://gitea.example.com/acme/app.git",
		ActiveStates: []string{"Todo", "In Progress"},
	}
	client := New("https://gitea.example.com", "token", []domain.ProjectConfig{project}, nil)

	_, err := client.FetchPendingIssues(context.Background(), project)
	if err == nil {
		t.Fatal("FetchPendingIssues returned nil error, want invalid active_states error")
	}
	if !strings.Contains(err.Error(), "active_states") || !strings.Contains(err.Error(), "open") {
		t.Fatalf("error = %q, want clear active_states guidance", err.Error())
	}
}

func TestFetchPendingIssuesUsesActiveStatesDedupesAndSkipsPullRequests(t *testing.T) {
	var states []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/acme/app/issues" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		state := r.URL.Query().Get("state")
		states = append(states, state)
		switch state {
		case "open":
			_ = json.NewEncoder(w).Encode([]giteaIssue{
				{Number: 10, Title: "ready"},
				{Number: 11, Title: "pull request", PullRequest: map[string]any{}},
			})
		case "closed":
			_ = json.NewEncoder(w).Encode([]giteaIssue{
				{Number: 10, Title: "ready duplicate"},
				{Number: 12, Title: "closed ready"},
			})
		default:
			t.Fatalf("unexpected state query %q", state)
		}
	}))
	defer server.Close()

	project := domain.ProjectConfig{
		ID:           "p",
		RepoURL:      "https://gitea.example.com/acme/app.git",
		ActiveStates: []string{"open", "OPEN", "closed"},
	}
	client := New(server.URL, "token", []domain.ProjectConfig{project}, server.Client())

	issues, err := client.FetchPendingIssues(context.Background(), project)
	if err != nil {
		t.Fatalf("FetchPendingIssues returned error: %v", err)
	}
	if !reflect.DeepEqual(states, []string{"open", "closed"}) {
		t.Fatalf("states = %#v, want open then closed", states)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %#v, want two pending issues", issues)
	}
	gotIDs := []string{issues[0].ID, issues[1].ID}
	if !reflect.DeepEqual(gotIDs, []string{"10", "12"}) {
		t.Fatalf("issue IDs = %#v, want 10 and 12", gotIDs)
	}
	for _, issue := range issues {
		if issue.ProjectID != "p" {
			t.Fatalf("issue ProjectID = %q, want p", issue.ProjectID)
		}
	}
}

func TestMarkStatusReplacesManagedLabelAndComments(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/repos/acme/app/labels":
			w.WriteHeader(http.StatusCreated)
		case "PUT /api/v1/repos/acme/app/issues/12/labels":
			var body struct {
				Labels []string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode labels request: %v", err)
			}
			if !reflect.DeepEqual(body.Labels, []string{"bug", "symphony-done"}) {
				t.Fatalf("labels body = %#v, want bug plus symphony-done", body.Labels)
			}
			w.WriteHeader(http.StatusOK)
		case "POST /api/v1/repos/acme/app/issues/12/comments":
			var body struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode comment request: %v", err)
			}
			for _, want := range []string{"symphony/p/issue-12-fix-login", "abc123"} {
				if !strings.Contains(body.Body, want) {
					t.Fatalf("comment body = %q, missing %q", body.Body, want)
				}
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	project := domain.ProjectConfig{ID: "p", RepoURL: "https://gitea.example.com/acme/app.git"}
	client := New(server.URL, "token", []domain.ProjectConfig{project}, server.Client())
	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Labels: []string{"bug", "symphony-running"}}

	result := domain.PublishResult{Branch: "symphony/p/issue-12-fix-login", Commit: "abc123"}
	if err := client.MarkStatus(context.Background(), issue, domain.StatusUpdate{
		Status:  domain.StatusDone,
		Publish: result,
	}); err != nil {
		t.Fatalf("MarkStatus returned error: %v", err)
	}

	want := []string{
		"POST /api/v1/repos/acme/app/labels",
		"PUT /api/v1/repos/acme/app/issues/12/labels",
		"POST /api/v1/repos/acme/app/issues/12/comments",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestMarkStatusFailedCommentIncludesFailureContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/repos/acme/app/labels":
			w.WriteHeader(http.StatusCreated)
		case "PUT /api/v1/repos/acme/app/issues/12/labels":
			var body struct {
				Labels []string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode labels request: %v", err)
			}
			if !reflect.DeepEqual(body.Labels, []string{"bug", "symphony-failed"}) {
				t.Fatalf("labels body = %#v, want bug plus symphony-failed", body.Labels)
			}
			w.WriteHeader(http.StatusOK)
		case "POST /api/v1/repos/acme/app/issues/12/comments":
			var body struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode comment request: %v", err)
			}
			for _, want := range []string{"reviewer failed: review failed", "C:\\workspaces\\p\\issue-12-fix-login"} {
				if !strings.Contains(body.Body, want) {
					t.Fatalf("comment body = %q, missing %q", body.Body, want)
				}
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	project := domain.ProjectConfig{ID: "p", RepoURL: "https://gitea.example.com/acme/app.git"}
	client := New(server.URL, "token", []domain.ProjectConfig{project}, server.Client())
	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Labels: []string{"bug", "symphony-running"}}

	if err := client.MarkStatus(context.Background(), issue, domain.StatusUpdate{
		Status:        domain.StatusFailed,
		FailureReason: "reviewer failed: review failed",
		WorkspacePath: "C:\\workspaces\\p\\issue-12-fix-login",
	}); err != nil {
		t.Fatalf("MarkStatus returned error: %v", err)
	}
}
