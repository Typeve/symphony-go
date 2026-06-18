package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/local/symphony/internal/domain"
	"gopkg.in/yaml.v3"
)

const (
	defaultPollInterval     = 30 * time.Second
	defaultMaxConcurrent    = 1
	defaultCodexCommand     = "codex"
	defaultCodexTimeout     = 30 * time.Minute
	defaultReviewerCommand  = "claude"
	defaultReviewerTimeout  = 15 * time.Minute
	defaultWorkspaceDirName = "symphony-workspaces"
)

// Load reads a YAML config file, expands ${ENV_VAR} references, and returns
// a fully resolved domain.Config.
func Load(path string) (domain.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Config{}, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg domain.Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return domain.Config{}, fmt.Errorf("parse config: %w", err)
	}
	applyDefaults(&cfg)
	if err := Validate(cfg); err != nil {
		return domain.Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Validate checks the minimal required fields for the Gitea-only MVP.
func Validate(cfg domain.Config) error {
	if strings.TrimSpace(cfg.Gitea.Endpoint) == "" {
		return fmt.Errorf("gitea.endpoint is required")
	}
	if strings.TrimSpace(cfg.Gitea.Token) == "" {
		return fmt.Errorf("gitea.token is required")
	}
	if len(cfg.Gitea.Projects) == 0 {
		return fmt.Errorf("gitea.projects must contain at least one project")
	}
	for i, project := range cfg.Gitea.Projects {
		if strings.TrimSpace(project.ID) == "" {
			return fmt.Errorf("gitea.projects[%d].id is required", i)
		}
		if strings.TrimSpace(project.RepoURL) == "" {
			return fmt.Errorf("gitea.projects[%d].repo_url is required", i)
		}
	}
	return nil
}

func applyDefaults(cfg *domain.Config) {
	if cfg.Scheduler.PollInterval <= 0 {
		cfg.Scheduler.PollInterval = defaultPollInterval
	}
	if cfg.Scheduler.MaxConcurrent <= 0 {
		cfg.Scheduler.MaxConcurrent = defaultMaxConcurrent
	}
	if strings.TrimSpace(cfg.Codex.Command) == "" {
		cfg.Codex.Command = defaultCodexCommand
	} else {
		cfg.Codex.Command = strings.TrimSpace(cfg.Codex.Command)
	}
	if cfg.Codex.Timeout <= 0 {
		cfg.Codex.Timeout = defaultCodexTimeout
	}
	if strings.TrimSpace(cfg.Reviewer.Command) == "" {
		cfg.Reviewer.Command = defaultReviewerCommand
	} else {
		cfg.Reviewer.Command = strings.TrimSpace(cfg.Reviewer.Command)
	}
	if cfg.Reviewer.Timeout <= 0 {
		cfg.Reviewer.Timeout = defaultReviewerTimeout
	}
	if strings.TrimSpace(cfg.Workspace.Root) == "" {
		cfg.Workspace.Root = filepath.Join(os.TempDir(), defaultWorkspaceDirName)
	} else {
		cfg.Workspace.Root = strings.TrimSpace(cfg.Workspace.Root)
	}
}
