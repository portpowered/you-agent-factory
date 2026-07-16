package worktree

import (
	"strings"
)

// ShouldPrepareFactoryWorktreeForCodex reports whether a model workstation dispatch
// should materialize a factory-local git worktree before execution. The historical
// name remains for compatibility. A resolved worktree is factory-managed even
// when the runtime uses its default provider, so model/provider selection cannot
// bypass isolation.
// A workstation with an authored workingDirectory retains CLI worktree passthrough.
func ShouldPrepareFactoryWorktreeForCodex(
	_ string,
	authoredWorkingDirectory string,
	resolvedWorktree string,
) bool {
	if strings.TrimSpace(resolvedWorktree) == "" {
		return false
	}
	if strings.TrimSpace(authoredWorkingDirectory) != "" {
		return false
	}
	return true
}
