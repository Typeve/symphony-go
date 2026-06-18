package agentcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/local/symphony/internal/agentenv"
	"github.com/local/symphony/internal/commandline"
)

const maxOutput = 1024

// Spec describes one agent command invocation.
type Spec struct {
	Command        string
	DefaultCommand string
	Timeout        time.Duration
	Workspace      string
	Prompt         string
}

// Run executes an agent command with prompt args, an allowlisted environment,
// timeout handling, and bounded failure diagnostics.
func Run(ctx context.Context, spec Spec) error {
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	name, args, err := commandline.Split(spec.Command, spec.DefaultCommand)
	if err != nil {
		return err
	}
	args = append(args, "--prompt", spec.Prompt)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = spec.Workspace
	cmd.Env = agentenv.Filter(os.Environ())

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		text := boundOutput(out.String())
		if text != "" {
			return fmt.Errorf("%s: %w: %s", name, err, text)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func boundOutput(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > maxOutput {
		return text[:maxOutput] + "...[truncated]"
	}
	return text
}
