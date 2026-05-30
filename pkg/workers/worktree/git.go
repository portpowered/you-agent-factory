package worktree

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitCommander runs git CLI commands. Tests inject fakes; production uses ExecGitCommander.
type GitCommander interface {
	Run(ctx context.Context, dir string, args ...string) (stdout string, stderr string, exitCode int, err error)
}

// ExecGitCommander implements GitCommander via the git binary on PATH.
type ExecGitCommander struct{}

func (ExecGitCommander) Run(ctx context.Context, dir string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return "", stderrBuf.String(), -1, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
	}
	return strings.TrimSpace(stdoutBuf.String()), strings.TrimSpace(stderrBuf.String()), exitCode, nil
}
