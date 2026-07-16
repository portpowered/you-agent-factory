package worktree

import (
	"strings"
)

// ShouldPrepareFactoryWorktreeForCodex reports whether a model workstation dispatch
// should materialize a factory-local git worktree before execution. The historical
// name remains for compatibility. A resolved worktree is factory-managed for every
// supported agent provider, so model/provider selection cannot bypass isolation.
// A workstation with an authored workingDirectory retains CLI worktree passthrough.
func ShouldPrepareFactoryWorktreeForCodex(
	executionModelProvider string,
	authoredWorkingDirectory string,
	resolvedWorktree string,
) bool {
	if strings.TrimSpace(resolvedWorktree) == "" {
		return false
	}
	if strings.TrimSpace(authoredWorkingDirectory) != "" {
		return false
	}
	return strings.TrimSpace(executionModelProvider) != ""
}
