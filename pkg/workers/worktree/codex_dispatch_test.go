package worktree

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestShouldPrepareFactoryWorktreeForCodex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		runnerID                 string
		authoredWorkingDirectory string
		resolvedWorktree         string
		want                     bool
	}{
		{
			name:             "CodexWithResolvedWorktree",
			runnerID:         interfaces.RunnerIDCodex,
			resolvedWorktree: "feature-a",
			want:             true,
		},
		{
			name:                     "SkipsWhenWorkingDirectoryAuthored",
			runnerID:                 interfaces.RunnerIDCodex,
			authoredWorkingDirectory: `/repo/{{ .Branch }}`,
			resolvedWorktree:         "feature-a",
			want:                     false,
		},
		{
			name:             "SkipsWithoutResolvedWorktree",
			runnerID:         interfaces.RunnerIDCodex,
			resolvedWorktree: "",
			want:             false,
		},
		{
			name:             "SkipsForNonCodexRunner",
			runnerID:         interfaces.RunnerIDCursorCLI,
			resolvedWorktree: "feature-a",
			want:             false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldPrepareFactoryWorktreeForCodex(tc.runnerID, tc.authoredWorkingDirectory, tc.resolvedWorktree)
			if got != tc.want {
				t.Fatalf("ShouldPrepareFactoryWorktreeForCodex() = %v, want %v", got, tc.want)
			}
		})
	}
}
