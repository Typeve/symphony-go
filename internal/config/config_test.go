package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadExpandsEnvironmentAndValidatesRequiredFields(t *testing.T) {
	t.Setenv("GITEA_TOKEN_FOR_TEST", "fixture-token")
	path := writeConfig(t, `
gitea:
  endpoint: "https://gitea.example.com"
  token: "${GITEA_TOKEN_FOR_TEST}"
  projects:
    - id: "app"
      repo_url: "https://gitea.example.com/acme/app.git"
      task_label: "symphony-task"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Gitea.Token != "fixture-token" {
		t.Fatal("Gitea token was not expanded from environment")
	}
	if len(cfg.Gitea.Projects) != 1 || cfg.Gitea.Projects[0].ID != "app" {
		t.Fatalf("projects = %#v, want configured app project", cfg.Gitea.Projects)
	}
	if cfg.Gitea.Projects[0].TaskLabel != "symphony-task" {
		t.Fatalf("task label = %q, want symphony-task", cfg.Gitea.Projects[0].TaskLabel)
	}
}

func TestLoadResolvesRuntimeDefaults(t *testing.T) {
	path := writeConfig(t, `
gitea:
  endpoint: "https://gitea.example.com"
  token: "fixture-token"
  projects:
    - id: "app"
      repo_url: "https://gitea.example.com/acme/app.git"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Scheduler.PollInterval != 30*time.Second {
		t.Fatalf("poll interval = %s, want 30s", cfg.Scheduler.PollInterval)
	}
	if cfg.Scheduler.MaxConcurrent != 1 {
		t.Fatalf("max concurrent = %d, want 1", cfg.Scheduler.MaxConcurrent)
	}
	if cfg.Codex.Command != "codex" || cfg.Codex.Timeout != 30*time.Minute {
		t.Fatalf("codex config = %#v, want default command and timeout", cfg.Codex)
	}
	if cfg.Reviewer.Command != "claude" || cfg.Reviewer.Timeout != 15*time.Minute {
		t.Fatalf("reviewer config = %#v, want default command and timeout", cfg.Reviewer)
	}
	wantRoot := filepath.Join(os.TempDir(), "symphony-workspaces")
	if cfg.Workspace.Root != wantRoot {
		t.Fatalf("workspace root = %q, want %q", cfg.Workspace.Root, wantRoot)
	}
}

func TestLoadPreservesConfiguredRuntimeValues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	path := writeConfig(t, `
gitea:
  endpoint: "https://gitea.example.com"
  token: "fixture-token"
  projects:
    - id: "app"
      repo_url: "https://gitea.example.com/acme/app.git"
scheduler:
  poll_interval: 5s
  max_concurrent: 2
codex:
  command: " codex app-server "
  timeout: 2m
reviewer:
  command: " claude --strict "
  timeout: 45s
workspace:
  root: ' `+root+` '
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Scheduler.PollInterval != 5*time.Second || cfg.Scheduler.MaxConcurrent != 2 {
		t.Fatalf("scheduler config = %#v, want configured values", cfg.Scheduler)
	}
	if cfg.Codex.Command != "codex app-server" || cfg.Codex.Timeout != 2*time.Minute {
		t.Fatalf("codex config = %#v, want configured values", cfg.Codex)
	}
	if cfg.Reviewer.Command != "claude --strict" || cfg.Reviewer.Timeout != 45*time.Second {
		t.Fatalf("reviewer config = %#v, want configured values", cfg.Reviewer)
	}
	if cfg.Workspace.Root != root {
		t.Fatalf("workspace root = %q, want %q", cfg.Workspace.Root, root)
	}
}

func TestLoadRejectsMissingGiteaEndpoint(t *testing.T) {
	path := writeConfig(t, `
gitea:
  token: "fixture-token"
  projects:
    - id: "app"
      repo_url: "https://gitea.example.com/acme/app.git"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error, want missing endpoint error")
	}
	if !strings.Contains(err.Error(), "gitea.endpoint") {
		t.Fatalf("error = %q, want gitea.endpoint", err.Error())
	}
}

func TestLoadRejectsProjectWithoutRepoURL(t *testing.T) {
	path := writeConfig(t, `
gitea:
  endpoint: "https://gitea.example.com"
  token: "fixture-token"
  projects:
    - id: "app"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error, want missing repo_url error")
	}
	if !strings.Contains(err.Error(), "repo_url") {
		t.Fatalf("error = %q, want repo_url", err.Error())
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "symphony.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
