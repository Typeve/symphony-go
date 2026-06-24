package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/local/symphony/internal/agentcmd"
	"github.com/local/symphony/internal/domain"
)

// RunCodex executes the configured Codex command inside the workspace.
func RunCodex(ctx context.Context, cfg domain.Config, issue domain.Issue, ws domain.Workspace) error {
	prompt := buildCodexPrompt(issue)
	if err := agentcmd.Run(ctx, agentcmd.Spec{
		Command:        cfg.Codex.Command,
		DefaultCommand: "codex",
		Args:           codexArgs(cfg.Codex.Model),
		Timeout:        cfg.Codex.Timeout,
		Workspace:      ws.Path,
		Prompt:         prompt,
	}); err != nil {
		return fmt.Errorf("codex: %w", err)
	}
	return nil
}

func codexArgs(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	return []string{"--model", model}
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
