package reviewer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/local/symphony/internal/agentcmd"
)

// Run invokes the Claude CLI to review code in the given workspace directory.
// command is the CLI binary (e.g. "claude"), timeout is the max duration, and
// workspace is the absolute path to the directory to review.
func Run(ctx context.Context, command string, timeout time.Duration, workspace string) error {
	reviewPrompt := "Review the changes in this repository for correctness, bugs, and code quality. Report your findings."

	slog.Info("running reviewer command",
		"command", command,
		"workspace", workspace,
		"timeout", timeout,
	)

	if err := agentcmd.Run(ctx, agentcmd.Spec{
		Command:        command,
		DefaultCommand: "claude",
		Timeout:        timeout,
		Workspace:      workspace,
		Prompt:         reviewPrompt,
	}); err != nil {
		return fmt.Errorf("reviewer failed: %w", err)
	}

	return nil
}
