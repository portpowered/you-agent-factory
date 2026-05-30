package worktree

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestShouldPrepareFactoryWorktreeForCodex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		executionModelProvider   string
		authoredWorkingDirectory string
		resolvedWorktree         string
		want                     bool
	}{
		{
			name:                   "CodexWithResolvedWorktree",
			executionModelProvider: string(interfaces.ModelProviderCodex),
			resolvedWorktree:       "feature-a",
			want:                   true,
		},
		{
			name:                     "SkipsWhenWorkingDirectoryAuthored",
			executionModelProvider:   string(interfaces.ModelProviderCodex),
			authoredWorkingDirectory: `/repo/{{ .Branch }}`,
			resolvedWorktree:         "feature-a",
			want:                     false,
		},
		{
			name:                   "SkipsWithoutResolvedWorktree",
			executionModelProvider: string(interfaces.ModelProviderCodex),
			resolvedWorktree:       "",
			want:                   false,
		},
		{
			name:                   "SkipsForNonCodexExecutionProvider",
			executionModelProvider: string(interfaces.ModelProviderClaude),
			resolvedWorktree:       "feature-a",
			want:                   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldPrepareFactoryWorktreeForCodex(tc.executionModelProvider, tc.authoredWorkingDirectory, tc.resolvedWorktree)
			if got != tc.want {
				t.Fatalf("ShouldPrepareFactoryWorktreeForCodex() = %v, want %v", got, tc.want)
			}
		})
	}
}
