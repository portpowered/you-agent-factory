package worktree

import (
	"context"
	"fmt"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// GitCommander is retained as the Worktree package's local spelling of the
// canonical Workers external-effect contract.
type GitCommander = workers.WorktreeGitCommander

// PlatformGitCommander adapts the composition-selected platform process runner
// to the exact Git command contract required by worktree preparation.
type PlatformGitCommander struct {
	runner platformprocess.CommandRunner
}

// NewPlatformGitCommander constructs the adapter without selecting a process
// implementation. Wire owns that selection.
func NewPlatformGitCommander(runner platformprocess.CommandRunner) (PlatformGitCommander, error) {
	if runner == nil {
		return PlatformGitCommander{}, fmt.Errorf("construct Worker worktree Git commander: process runner is required")
	}
	return PlatformGitCommander{runner: runner}, nil
}

func (commander PlatformGitCommander) Run(
	ctx context.Context,
	dir string,
	args ...string,
) (string, string, int, error) {
	if commander.runner == nil {
		return "", "", -1, fmt.Errorf("Worker worktree Git process runner is required")
	}
	result, err := commander.runner.Run(ctx, platformprocess.CommandRequest{
		Command: "git",
		Args:    append([]string(nil), args...),
		WorkDir: dir,
	})
	if err != nil {
		return "", strings.TrimSpace(string(result.Stderr)), result.ExitCode,
			fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(result.Stdout)), strings.TrimSpace(string(result.Stderr)), result.ExitCode, nil
}

var _ workers.WorktreeGitCommander = PlatformGitCommander{}
