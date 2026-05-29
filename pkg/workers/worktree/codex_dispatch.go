package worktree

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ShouldPrepareFactoryWorktreeForCodex reports whether a Codex model workstation
// dispatch should materialize a factory-local git worktree before execution.
// Preparation runs when a worktree template resolved to a name and the
// workstation did not also author a workingDirectory template.
func ShouldPrepareFactoryWorktreeForCodex(
	runnerID string,
	authoredWorkingDirectory string,
	resolvedWorktree string,
) bool {
	if strings.TrimSpace(resolvedWorktree) == "" {
		return false
	}
	if strings.TrimSpace(authoredWorkingDirectory) != "" {
		return false
	}
	return interfaces.NormalizeRunnerID(runnerID) == interfaces.RunnerIDCodex
}
