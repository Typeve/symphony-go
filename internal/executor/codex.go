package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/local/symphony/internal/agentenv"
	"github.com/local/symphony/internal/commandline"
	"github.com/local/symphony/internal/domain"
)

// RunCodex executes the configured Codex command inside the workspace.
func RunCodex(ctx context.Context, cfg domain.Config, issue domain.Issue, ws domain.Workspace) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.Codex.Timeout)
	defer cancel()

	prompt := buildCodexPrompt(issue)

	name, args, err := commandline.Split(cfg.Codex.Command, "codex")
	if err != nil {
		return err
	}
	args = append(args, "--prompt", prompt)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = ws.Path
	cmd.Env = agentenv.Filter(os.Environ())

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(out.String())
		if len(text) > 1024 {
			text = text[:1024] + "...[truncated]"
		}
		if text != "" {
			return fmt.Errorf("codex: %w: %s", err, text)
		}
		return fmt.Errorf("codex: %w", err)
	}
	return nil
}

func buildCodexPrompt(issue domain.Issue) string {
	var b strings.Builder
	b.WriteString("Resolve the following issue:\n\n")
	b.WriteString(fmt.Sprintf("Title: %s\n", issue.Title))
	if issue.Description != nil && *issue.Description != "" {
		b.WriteString(fmt.Sprintf("Description:\n%s\n", *issue.Description))
	}
	b.WriteString("\nImplement the necessary changes in this repository.")
	return b.String()
}
