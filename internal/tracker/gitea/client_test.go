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

func TestFetchIssuesSkipsManagedStatusLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/acme/app/issues" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("state"); got != "open" {
			t.Fatalf("state query = %q, want open", got)
		}
		_ = json.NewEncoder(w).Encode([]giteaIssue{
			{Number: 1, Title: "ready", State: "open"},
			{Number: 2, Title: "running", State: "open", Labels: []giteaLabel{{Name: "symphony-running"}}},
			{Number: 3, Title: "done", State: "open", Labels: []giteaLabel{{Name: "symphony-done"}}},
			{Number: 4, Title: "failed", State: "open", Labels: []giteaLabel{{Name: "symphony-failed"}}},
		})
	}))
	defer server.Close()

	project := domain.ProjectConfig{ID: "p", RepoURL: "https://gitea.example.com/acme/app.git"}
	client := New(server.URL, "token", []domain.ProjectConfig{project}, server.Client())

	issues, err := client.FetchIssues(context.Background(), project)
	if err != nil {
		t.Fatalf("FetchIssues returned error: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "1" {
		t.Fatalf("issues = %#v, want only issue 1", issues)
	}
}

func TestFetchIssuesRejectsInvalidActiveStates(t *testing.T) {
	project := domain.ProjectConfig{
		ID:           "p",
		RepoURL:      "https://gitea.example.com/acme/app.git",
		ActiveStates: []string{"Todo", "In Progress"},
	}
	client := New("https://gitea.example.com", "token", []domain.ProjectConfig{project}, nil)

	_, err := client.FetchIssues(context.Background(), project)
	if err == nil {
		t.Fatal("FetchIssues returned nil error, want invalid active_states error")
	}
	if !strings.Contains(err.Error(), "active_states") || !strings.Contains(err.Error(), "open") {
		t.Fatalf("error = %q, want clear active_states guidance", err.Error())
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
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	project := domain.ProjectConfig{ID: "p", RepoURL: "https://gitea.example.com/acme/app.git"}
	client := New(server.URL, "token", []domain.ProjectConfig{project}, server.Client())
	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Labels: []string{"bug", "symphony-running"}}

	if err := client.MarkStatus(context.Background(), issue, domain.StatusDone); err != nil {
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
