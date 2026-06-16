package reviewer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/local/symphony/internal/agentenv"
)

// Run invokes the Claude CLI to review code in the given workspace directory.
// command is the CLI binary (e.g. "claude"), timeout is the max duration, and
// workspace is the absolute path to the directory to review.
func Run(ctx context.Context, command string, timeout time.Duration, workspace string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "claude"
	}

	if timeout <= 0 {
		timeout = 15 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reviewPrompt := "Review the changes in this repository for correctness, bugs, and code quality. Report your findings."

	cmd := exec.CommandContext(ctx, command, "--prompt", reviewPrompt)
	cmd.Dir = workspace
	cmd.Env = agentenv.Filter(os.Environ())

	slog.Info("running reviewer command",
		"command", command,
		"workspace", workspace,
		"timeout", timeout,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		text := strings.TrimSpace(string(out))
		if len(text) > 1024 {
			text = text[:1024] + "...[truncated]"
		}
		return fmt.Errorf("reviewer %s failed: %w: %s", command, err, text)
	}

	return nil
}
