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
	"github.com/local/symphony/internal/commandline"
)

// Run invokes the Claude CLI to review code in the given workspace directory.
// command is the CLI binary (e.g. "claude"), timeout is the max duration, and
// workspace is the absolute path to the directory to review.
func Run(ctx context.Context, command string, timeout time.Duration, workspace string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reviewPrompt := "Review the changes in this repository for correctness, bugs, and code quality. Report your findings."

	name, args, err := commandline.Split(command, "claude")
	if err != nil {
		return err
	}
	args = append(args, "--prompt", reviewPrompt)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workspace
	cmd.Env = agentenv.Filter(os.Environ())

	slog.Info("running reviewer command",
		"command", name,
		"workspace", workspace,
		"timeout", timeout,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		text := strings.TrimSpace(string(out))
		if len(text) > 1024 {
			text = text[:1024] + "...[truncated]"
		}
		return fmt.Errorf("reviewer %s failed: %w: %s", name, err, text)
	}

	return nil
}
