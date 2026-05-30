package worktree

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ShouldPrepareFactoryWorktreeForCodex reports whether a Codex model workstation
// dispatch should materialize a factory-local git worktree before execution.
// Preparation runs when execution will use the Codex provider, a worktree template
// resolved to a name, and the workstation did not also author a workingDirectory
// template. Callers must pass the resolved execution model provider (not runner ID
// alone) so legacy modelProvider: claude workstations with default runner selection
// keep CLI --worktree passthrough.
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
	return strings.TrimSpace(executionModelProvider) == string(interfaces.ModelProviderCodex)
}
