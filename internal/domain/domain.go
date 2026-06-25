package domain

import "time"

// Issue represents a tracker issue.
type Issue struct {
	ProjectID   string
	ID          string
	Identifier  string
	Title       string
	Description *string
	Labels      []string
}

// ProjectConfig holds per-project settings.
type ProjectConfig struct {
	ID           string   `yaml:"id"`
	RepoURL      string   `yaml:"repo_url"`
	ActiveStates []string `yaml:"active_states"`
	TaskLabel    string   `yaml:"task_label"`
}

// Config is the top-level configuration loaded from YAML.
type Config struct {
	Gitea struct {
		Endpoint string          `yaml:"endpoint"`
		Token    string          `yaml:"token"`
		Projects []ProjectConfig `yaml:"projects"`
	} `yaml:"gitea"`

	Scheduler struct {
		PollInterval  time.Duration `yaml:"poll_interval"`
		MaxConcurrent int           `yaml:"max_concurrent"`
	} `yaml:"scheduler"`

	Codex struct {
		Command string        `yaml:"command"`
		Model   string        `yaml:"model"`
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"codex"`

	Reviewer struct {
		Command string        `yaml:"command"`
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"reviewer"`

	Workspace struct {
		Root string `yaml:"root"`
	} `yaml:"workspace"`
}

// Status represents a managed Symphony status label written to the tracker.
type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// Workspace represents a local working directory for an issue.
type Workspace struct {
	Path     string
	IssueKey string
}

// PublishResult holds the result of publishing an execution branch.
type PublishResult struct {
	Branch string
	Commit string
}

// StatusUpdate is the tracker-facing status write payload.
type StatusUpdate struct {
	Status        Status
	Publish       PublishResult
	FailureReason string
	WorkspacePath string
}
