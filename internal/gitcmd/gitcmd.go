package gitcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxOutput = 1024

// Options configures one Git command execution.
type Options struct {
	Dir    string
	Token  string
	Redact []string
}

// Run executes git with bounded, redacted failure diagnostics.
func Run(ctx context.Context, opts Options, args ...string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = opts.Dir
	cmd.Env = os.Environ()

	redactions := append([]string{}, opts.Redact...)
	if strings.TrimSpace(opts.Token) != "" {
		env, cleanup, err := askpassEnv(opts.Token)
		if err != nil {
			return err
		}
		defer cleanup()
		cmd.Env = append(cmd.Env, env...)
		redactions = append(redactions, opts.Token)
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		command := redact(strings.Join(args, " "), redactions)
		text := redactAndBound(out.String(), redactions)
		return fmt.Errorf("git %s failed: %w: %s", command, err, text)
	}
	return nil
}

func askpassEnv(token string) ([]string, func(), error) {
	dir, err := os.MkdirTemp("", "symphony-git-askpass-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create askpass dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	script := "#!/bin/sh\ncase \"$1\" in *Username*) printf 'oauth2\\n' ;; *) printf '%s\\n' \"$SYMPHONY_GIT_TOKEN\" ;; esac\n"
	path := dir + "/askpass.sh"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write askpass script: %w", err)
	}

	return []string{
		"GIT_ASKPASS=" + path,
		"GIT_TERMINAL_PROMPT=0",
		"SYMPHONY_GIT_TOKEN=" + token,
	}, cleanup, nil
}

func redactAndBound(text string, redactions []string) string {
	text = strings.TrimRight(text, "\n")
	text = redact(text, redactions)
	if len(text) > maxOutput {
		return text[:maxOutput] + "\n[truncated]"
	}
	return text
}

func redact(text string, redactions []string) string {
	for _, value := range redactions {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[REDACTED]")
		}
	}
	return text
}
