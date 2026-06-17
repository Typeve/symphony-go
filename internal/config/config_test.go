package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
