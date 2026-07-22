package worktree

import (
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
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
			executionModelProvider: string(modelprovider.ProviderCodex),
			resolvedWorktree:       "feature-a",
			want:                   true,
		},
		{
			name:                     "SkipsWhenWorkingDirectoryAuthored",
			executionModelProvider:   string(modelprovider.ProviderCodex),
			authoredWorkingDirectory: `/repo/{{ .Branch }}`,
			resolvedWorktree:         "feature-a",
			want:                     false,
		},
		{
			name:                   "SkipsWithoutResolvedWorktree",
			executionModelProvider: string(modelprovider.ProviderCodex),
			resolvedWorktree:       "",
			want:                   false,
		},
		{
			name:                   "SkipsForNonCodexExecutionProvider",
			executionModelProvider: string(modelprovider.ProviderClaude),
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
